package sessions

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/fit"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/statemigrate"
)

type Options struct {
	ContextLength int  `json:"context_length,omitempty"`
	GPULayers     any  `json:"gpu_layers,omitempty"`
	Force         bool `json:"force,omitempty"`
}
type CreateRequest struct {
	Model   string  `json:"model"`
	Compute string  `json:"compute,omitempty"`
	Options Options `json:"options,omitempty"`
}
type Manager struct {
	mu       sync.Mutex
	catalog  catalog.Catalog
	models   *models.Manager
	registry *backruntime.Registry
	paths    config.Paths
	local    compute.Local
	targets  map[string]compute.Target
	sessions map[string]*managed
	sink     events.Sink
	stateErr error
}
type managed struct {
	public  *backruntime.Session
	adapter backruntime.Adapter
	model   *models.Installed
}

func New(c catalog.Catalog, mm *models.Manager, registry *backruntime.Registry, paths config.Paths) *Manager {
	return NewWithEvents(c, mm, registry, paths, nil)
}

func NewWithEvents(c catalog.Catalog, mm *models.Manager, registry *backruntime.Registry, paths config.Paths, sink events.Sink) *Manager {
	local := compute.Local{}
	m := &Manager{catalog: c, models: mm, registry: registry, paths: paths, local: local, targets: map[string]compute.Target{"local": local}, sessions: map[string]*managed{}, sink: sink}
	m.stateErr = m.reconcile()
	return m
}

func (m *Manager) RegisterTarget(target compute.Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targets[target.Name()] = target
}

func (m *Manager) Create(ctx context.Context, request CreateRequest) (*backruntime.Session, error) {
	if m.stateErr != nil {
		return nil, m.stateErr
	}
	installed, adapter, computeTarget, err := m.resolve(ctx, request.Model, request.Compute, request.Options.Force)
	if err != nil {
		return nil, err
	}
	if err = checkFit(ctx, installed, computeTarget, request.Options.Force); err != nil {
		return nil, err
	}
	if err = adapter.Prepare(ctx, installed, computeTarget); err != nil {
		events.Emit(m.sink, events.Event{Type: events.RuntimeError, Kind: events.Warning, Message: err.Error()})
		return nil, err
	}
	events.Emit(m.sink, events.Event{Type: events.RuntimeStarting, Kind: events.Status, Message: "Starting " + adapter.Name()})
	gpu, err := gpuLayers(request.Options.GPULayers)
	if err != nil {
		events.Emit(m.sink, events.Event{Type: events.RuntimeError, Kind: events.Warning, Message: err.Error()})
		return nil, err
	}
	session, err := adapter.Start(ctx, installed, computeTarget, backruntime.StartOptions{Host: "127.0.0.1", ContextSize: request.Options.ContextLength, GPULayers: gpu})
	if err != nil {
		events.Emit(m.sink, events.Event{Type: events.RuntimeError, Kind: events.Warning, Message: err.Error()})
		return nil, err
	}
	session.ID = newID()
	m.mu.Lock()
	m.sessions[session.ID] = &managed{session, adapter, installed}
	m.persistLocked()
	m.mu.Unlock()
	go m.monitor(session.ID, session.Process)
	events.Emit(m.sink, events.Event{Type: events.RuntimeReady, Kind: events.Complete, Message: "Ready " + session.ModelID})
	return clone(session), nil
}

func (m *Manager) Transcribe(ctx context.Context, model, computeName string, request backruntime.TranscriptionRequest) (*backruntime.Transcription, error) {
	installed, adapter, target, err := m.resolve(ctx, model, computeName, request.Force)
	if err != nil {
		return nil, err
	}
	if err = checkFit(ctx, installed, target, request.Force); err != nil {
		return nil, err
	}
	if transcriber, ok := adapter.(backruntime.Transcriber); ok {
		if err := adapter.Prepare(ctx, installed, target); err != nil {
			return nil, err
		}
		return transcriber.Transcribe(ctx, installed, target, request)
	}
	if transcriber, ok := adapter.(backruntime.SessionTranscriber); ok && hasCapability(adapter, "transcription") {
		owned, err := m.ensureManaged(ctx, installed.ID, target.Name(), request.Force)
		if err != nil {
			return nil, err
		}
		return transcriber.TranscribeSession(ctx, owned.public, request)
	}
	return nil, fmt.Errorf("model %q uses runtime %q, which does not support transcription", installed.ID, adapter.Name())
}

func (m *Manager) Synthesize(ctx context.Context, model, computeName string, request backruntime.SpeechRequest) (*backruntime.Speech, error) {
	installed, adapter, target, err := m.resolve(ctx, model, computeName, request.Force)
	if err != nil {
		return nil, err
	}
	if err = checkFit(ctx, installed, target, request.Force); err != nil {
		return nil, err
	}
	synthesizer, ok := adapter.(backruntime.SessionSynthesizer)
	if !ok || !hasCapability(adapter, "speech") {
		return nil, fmt.Errorf("model %q uses runtime %q, which does not support speech synthesis", installed.ID, adapter.Name())
	}
	owned, err := m.ensureManaged(ctx, installed.ID, target.Name(), request.Force)
	if err != nil {
		return nil, err
	}
	return synthesizer.SynthesizeSession(ctx, owned.public, request)
}

func (m *Manager) ensureManaged(ctx context.Context, model, computeName string, force bool) (*managed, error) {
	m.mu.Lock()
	for _, item := range m.sessions {
		if item.public.ModelID == model && item.public.Compute == computeName && item.public.Status == "ready" && item.adapter != nil {
			m.mu.Unlock()
			return item, nil
		}
	}
	m.mu.Unlock()
	session, err := m.Create(ctx, CreateRequest{Model: model, Compute: computeName, Options: Options{Force: force}})
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.sessions[session.ID]
	if !ok {
		return nil, fmt.Errorf("created session %q is not owned", session.ID)
	}
	return item, nil
}

func hasCapability(adapter backruntime.Adapter, capability string) bool {
	for _, item := range adapter.Capabilities() {
		if strings.EqualFold(item, capability) {
			return true
		}
	}
	return false
}

func checkFit(ctx context.Context, installed *models.Installed, target compute.Target, force bool) error {
	if force {
		return nil
	}
	hardware, err := target.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect compute target for model fit: %w", err)
	}
	return fit.Refusal(fit.Evaluate(installed.Package, hardware))
}

func (m *Manager) resolve(ctx context.Context, model, targetName string, force bool) (*models.Installed, backruntime.Adapter, compute.Target, error) {
	entry, err := m.catalog.Resolve(model)
	if err != nil {
		return nil, nil, nil, err
	}
	if targetName == "" {
		targetName = "local"
	}
	m.mu.Lock()
	target, ok := m.targets[targetName]
	m.mu.Unlock()
	if !ok && targetName != "local" {
		if saved, loadErr := compute.NewTargetStore(m.paths).Get(targetName); loadErr == nil {
			target = compute.NewSSH(saved)
			if sshTarget, isSSH := target.(*compute.SSHTarget); isSSH {
				sshTarget.Sink = m.sink
			}
			m.RegisterTarget(target)
			ok = true
		}
	}
	if !ok {
		return nil, nil, nil, fmt.Errorf("compute target %q is not configured", targetName)
	}
	installed, err := m.models.Installed(entry.ID)
	if err != nil {
		candidate, resolveErr := m.models.ResolvePackage(ctx, entry)
		if resolveErr != nil {
			return nil, nil, nil, fmt.Errorf("resolve model %s: %w", entry.ID, resolveErr)
		}
		if !force {
			if fitErr := checkFit(ctx, candidate, target, false); fitErr != nil {
				return nil, nil, nil, fitErr
			}
		}
		installed, err = m.models.Pull(ctx, entry, m.sink)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("install model %s: %w", entry.ID, err)
		}
	} else if verifyErr := m.models.Verify(installed); verifyErr != nil {
		events.Emit(m.sink, events.Event{Type: events.ModelVerifyStarted, Kind: events.Warning, Message: "Installed model failed verification; repairing package"})
		installed, err = m.models.Pull(ctx, entry, m.sink)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("repair model %s after %v: %w", entry.ID, verifyErr, err)
		}
	}
	adapter, err := m.registry.Select(installed.Runtime)
	if err != nil {
		return nil, nil, nil, err
	}
	return installed, adapter, target, nil
}

func (m *Manager) Ensure(ctx context.Context, model string) (*backruntime.Session, error) {
	entry, err := m.catalog.Resolve(model)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	for _, s := range m.sessions {
		if s.public.ModelID == entry.ID && s.public.Status == "ready" {
			out := clone(s.public)
			m.mu.Unlock()
			return out, nil
		}
	}
	m.mu.Unlock()
	return m.Create(ctx, CreateRequest{Model: model, Compute: "local", Options: Options{GPULayers: "auto"}})
}
func (m *Manager) List() []*backruntime.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*backruntime.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, clone(s.public))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
func (m *Manager) Get(id string) (*backruntime.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stateErr != nil {
		return nil, m.stateErr
	}
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q was not found", id)
	}
	return clone(s.public), nil
}
func (m *Manager) Endpoint(id string) (string, error) {
	s, err := m.Get(id)
	if err != nil {
		return "", err
	}
	if s.Status != "ready" {
		return "", fmt.Errorf("session %q is %s", id, s.Status)
	}
	return s.Endpoint, nil
}
func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	if m.stateErr != nil {
		err := m.stateErr
		m.mu.Unlock()
		return err
	}
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %q was not found", id)
	}
	if s.adapter == nil || s.public.Process == nil {
		m.mu.Unlock()
		return fmt.Errorf("session %q is not owned by this runtime", id)
	}
	s.public.Status = "stopping"
	m.persistLocked()
	m.mu.Unlock()
	err := s.adapter.Stop(ctx, s.public)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		s.public.Status = "failed"
		s.public.LastError = err.Error()
	} else {
		s.public.Status = "stopped"
	}
	m.persistLocked()
	return err
}
func (m *Manager) Shutdown(ctx context.Context) {
	for _, s := range m.List() {
		if s.Status == "ready" || s.Status == "starting" {
			_ = m.Stop(ctx, s.ID)
		}
	}
}

func (m *Manager) monitor(id string, process compute.Process) {
	err := process.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || s.public.Status == "stopping" || s.public.Status == "stopped" {
		return
	}
	s.public.Status = "failed"
	if err != nil {
		s.public.LastError = err.Error()
	} else {
		s.public.LastError = "runtime process exited unexpectedly"
	}
	m.persistLocked()
}

func gpuLayers(v any) (int, error) {
	if v == nil {
		return 99, nil
	}
	switch x := v.(type) {
	case string:
		if strings.EqualFold(x, "auto") {
			return 99, nil
		}
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, fmt.Errorf("gpu_layers must be auto or an integer")
		}
		return n, nil
	case float64:
		return int(x), nil
	case int:
		return x, nil
	default:
		return 0, fmt.Errorf("gpu_layers must be auto or an integer")
	}
}
func clone(s *backruntime.Session) *backruntime.Session { x := *s; x.Process = nil; return &x }
func newID() string {
	var value [6]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("session-%d", os.Getpid())
	}
	return fmt.Sprintf("sess-%x", value)
}
func (m *Manager) statePath() string { return filepath.Join(m.paths.State, "sessions.json") }
func (m *Manager) StateError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stateErr
}
func (m *Manager) persistLocked() {
	items := make([]*backruntime.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		items = append(items, clone(s.public))
	}
	_ = statemigrate.WriteList(m.statePath(), "sessions", items)
}
func (m *Manager) reconcile() error {
	prior, err := statemigrate.ReadList[*backruntime.Session](m.statePath(), "sessions")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read session state: %w", err)
	}
	for _, s := range prior {
		if s.Status == "ready" || s.Status == "starting" || s.Status == "stopping" {
			s.Status = "failed"
			s.LastError = "runtime service restarted; the prior child process is no longer owned"
		}
		s.Process = nil
		m.sessions[s.ID] = &managed{public: s}
	}
	m.persistLocked()
	return nil
}
