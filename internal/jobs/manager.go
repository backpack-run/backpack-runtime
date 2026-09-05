package jobs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/artifacts"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/events"
)

type State string

const (
	Queued    State = "queued"
	Preparing State = "preparing"
	Loading   State = "loading"
	Running   State = "running"
	Completed State = "completed"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

type Input struct {
	Prompt string `json:"prompt"`
	Image  string `json:"image,omitempty"`
}
type Options struct {
	Width  int     `json:"width,omitempty"`
	Height int     `json:"height,omitempty"`
	Steps  int     `json:"steps,omitempty"`
	Frames int     `json:"frames,omitempty"`
	Seed   *int64  `json:"seed,omitempty"`
	FPS    float64 `json:"fps,omitempty"`
}
type CreateRequest struct {
	Model      string  `json:"model"`
	Capability string  `json:"capability"`
	Compute    string  `json:"compute,omitempty"`
	Input      Input   `json:"input"`
	Options    Options `json:"options,omitempty"`
}
type Job struct {
	ID             string               `json:"id"`
	Model          string               `json:"model"`
	Capability     string               `json:"capability"`
	RuntimeAdapter string               `json:"runtime_adapter"`
	Compute        string               `json:"compute"`
	Status         State                `json:"status"`
	CreatedAt      time.Time            `json:"created_at"`
	StartedAt      *time.Time           `json:"started_at,omitempty"`
	EndedAt        *time.Time           `json:"ended_at,omitempty"`
	Progress       *Progress            `json:"progress,omitempty"`
	Artifacts      []artifacts.Artifact `json:"artifacts,omitempty"`
	Error          string               `json:"error,omitempty"`
}
type Progress struct {
	Step    int    `json:"step,omitempty"`
	Total   int    `json:"total,omitempty"`
	Message string `json:"message,omitempty"`
}
type Reporter func(State, Progress)
type Runner interface {
	Capability() string
	Runtime() string
	Run(context.Context, string, CreateRequest, Reporter) ([]artifacts.Artifact, error)
}
type managed struct {
	public *Job
	cancel context.CancelFunc
}
type Manager struct {
	mu      sync.Mutex
	catalog catalog.Catalog
	paths   config.Paths
	runners map[string]Runner
	jobs    map[string]*managed
	sink    events.Sink
}

func New(c catalog.Catalog, paths config.Paths, sink events.Sink, runners ...Runner) *Manager {
	m := &Manager{catalog: c, paths: paths, runners: map[string]Runner{}, jobs: map[string]*managed{}, sink: sink}
	for _, runner := range runners {
		m.runners[strings.ToLower(runner.Capability())] = runner
	}
	m.reconcile()
	return m
}

func (m *Manager) Create(ctx context.Context, request CreateRequest) (*Job, error) {
	entry, err := m.catalog.Resolve(request.Model)
	if err != nil {
		return nil, err
	}
	request.Capability = strings.ToLower(strings.TrimSpace(request.Capability))
	if request.Compute == "" {
		request.Compute = "local"
	}
	if !modelCapability(entry, request.Capability) {
		return nil, fmt.Errorf("model %q does not declare capability %q", entry.ID, request.Capability)
	}
	if err = validateRequest(request); err != nil {
		return nil, err
	}
	runner, ok := m.runners[request.Capability]
	if !ok {
		return nil, fmt.Errorf("capability %q has no execution-validated runtime adapter in this build", request.Capability)
	}
	now := time.Now().UTC()
	job := &Job{ID: newID(), Model: entry.ID, Capability: request.Capability, RuntimeAdapter: runner.Runtime(), Compute: request.Compute, Status: Queued, CreatedAt: now}
	runContext, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.jobs[job.ID] = &managed{public: job, cancel: cancel}
	m.persistLocked()
	m.mu.Unlock()
	m.emit(job, events.JobCreated, events.Status, "Job queued", Progress{})
	go m.run(runContext, runner, job.ID, request)
	return clone(job), nil
}

func (m *Manager) run(ctx context.Context, runner Runner, id string, request CreateRequest) {
	m.update(id, Preparing, Progress{Message: "Preparing runtime"})
	reporter := func(state State, progress Progress) { m.update(id, state, progress) }
	items, err := runner.Run(ctx, id, request, reporter)
	m.mu.Lock()
	item, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	item.public.EndedAt = &now
	if ctx.Err() != nil {
		item.public.Status = Cancelled
		item.public.Error = "job cancelled"
	} else if err != nil {
		item.public.Status = Failed
		item.public.Error = err.Error()
	} else {
		item.public.Status = Completed
		item.public.Artifacts = items
	}
	result := clone(item.public)
	m.persistLocked()
	m.mu.Unlock()
	typeName, kind := events.JobCompleted, events.Complete
	if result.Status == Failed {
		typeName, kind = events.JobFailed, events.Warning
	}
	if result.Status == Cancelled {
		typeName, kind = events.JobCancelled, events.Warning
	}
	m.emit(result, typeName, kind, string(result.Status), Progress{})
}

func (m *Manager) update(id string, state State, progress Progress) {
	m.mu.Lock()
	item, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if item.public.StartedAt == nil {
		now := time.Now().UTC()
		item.public.StartedAt = &now
	}
	item.public.Status = state
	item.public.Progress = &progress
	result := clone(item.public)
	m.persistLocked()
	m.mu.Unlock()
	typeName := events.JobProgress
	if state == Preparing {
		typeName = events.JobPreparing
	}
	if state == Loading {
		typeName = events.JobModelLoad
	}
	m.emit(result, typeName, events.Progress, progress.Message, progress)
}

func (m *Manager) List() []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Job, 0, len(m.jobs))
	for _, item := range m.jobs {
		out = append(out, clone(item.public))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
func (m *Manager) Get(id string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %q was not found", id)
	}
	return clone(item.public), nil
}
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	item, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("job %q was not found", id)
	}
	if item.public.Status == Completed || item.public.Status == Failed || item.public.Status == Cancelled {
		m.mu.Unlock()
		return fmt.Errorf("job %q is already %s", id, item.public.Status)
	}
	cancel := item.cancel
	m.mu.Unlock()
	cancel()
	return nil
}

func (m *Manager) ArtifactPath(jobID, artifactID string) (string, error) {
	job, err := m.Get(jobID)
	if err != nil {
		return "", err
	}
	return (artifacts.Manager{Root: m.paths.Outputs}).Resolve(jobID, artifactID, job.Artifacts)
}

func validateRequest(r CreateRequest) error {
	if strings.TrimSpace(r.Input.Prompt) == "" {
		return fmt.Errorf("input.prompt is required")
	}
	if len(r.Input.Prompt) > 12000 {
		return fmt.Errorf("input.prompt exceeds 12000 characters")
	}
	if r.Options.Width < 0 || r.Options.Height < 0 || r.Options.Width > 4096 || r.Options.Height > 4096 {
		return fmt.Errorf("width and height must be at most 4096")
	}
	if r.Options.Steps < 0 || r.Options.Steps > 200 {
		return fmt.Errorf("steps must be between 1 and 200 when specified")
	}
	if r.Options.Frames < 0 || r.Options.Frames > 1000 {
		return fmt.Errorf("frames must be at most 1000")
	}
	if r.Options.FPS < 0 || r.Options.FPS > 120 {
		return fmt.Errorf("fps must be at most 120")
	}
	if r.Capability == "image-generation" && r.Input.Image != "" {
		return fmt.Errorf("image-generation does not accept an image input")
	}
	return nil
}
func modelCapability(model catalog.Model, capability string) bool {
	for _, item := range model.Capabilities {
		if strings.EqualFold(item, capability) {
			return true
		}
	}
	return false
}
func clone(job *Job) *Job {
	copy := *job
	copy.Artifacts = append([]artifacts.Artifact(nil), job.Artifacts...)
	if job.Progress != nil {
		p := *job.Progress
		copy.Progress = &p
	}
	return &copy
}
func newID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("job-%x", value)
}
func (m *Manager) statePath() string { return filepath.Join(m.paths.State, "jobs.json") }
func (m *Manager) persistLocked() {
	items := make([]*Job, 0, len(m.jobs))
	for _, x := range m.jobs {
		items = append(items, clone(x.public))
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	_ = os.MkdirAll(m.paths.State, 0700)
	tmp := m.statePath() + ".tmp"
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = os.Remove(m.statePath())
		_ = os.Rename(tmp, m.statePath())
	}
}
func (m *Manager) reconcile() {
	data, err := os.ReadFile(m.statePath())
	if err != nil {
		return
	}
	var prior []*Job
	if json.Unmarshal(data, &prior) != nil {
		return
	}
	for _, job := range prior {
		if job.Status == Queued || job.Status == Preparing || job.Status == Loading || job.Status == Running {
			job.Status = Failed
			job.Error = "runtime service restarted while the job was active"
			now := time.Now().UTC()
			job.EndedAt = &now
		}
		m.jobs[job.ID] = &managed{public: job, cancel: func() {}}
	}
	m.persistLocked()
}
func (m *Manager) emit(job *Job, eventType string, kind events.Kind, message string, p Progress) {
	events.Emit(m.sink, events.Event{Type: eventType, Kind: kind, Message: message, JobID: job.ID, ModelID: job.Model, Current: int64(p.Step), Total: int64(p.Total)})
}
