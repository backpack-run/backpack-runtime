package sessions

import (
	"context"
	"crypto/rand"
	"encoding/json"
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
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
)

type Options struct {
	ContextLength int `json:"context_length,omitempty"`
	GPULayers     any `json:"gpu_layers,omitempty"`
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
}
type managed struct {
	public  *backruntime.Session
	adapter backruntime.Adapter
	model   *models.Installed
}

func New(c catalog.Catalog, mm *models.Manager, registry *backruntime.Registry, paths config.Paths) *Manager {
	local := compute.Local{}
	m := &Manager{catalog: c, models: mm, registry: registry, paths: paths, local: local, targets: map[string]compute.Target{"local": local}, sessions: map[string]*managed{}}
	m.reconcile()
	return m
}

func (m *Manager) RegisterTarget(target compute.Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targets[target.Name()] = target
}

func (m *Manager) Create(ctx context.Context, request CreateRequest) (*backruntime.Session, error) {
	entry, err := m.catalog.Resolve(request.Model)
	if err != nil {
		return nil, err
	}
	target := request.Compute
	if target == "" {
		target = "local"
	}
	m.mu.Lock()
	computeTarget, ok := m.targets[target]
	m.mu.Unlock()
	if !ok && target != "local" {
		if saved, err := compute.NewTargetStore(m.paths).Get(target); err == nil {
			computeTarget = compute.NewSSH(saved)
			m.RegisterTarget(computeTarget)
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("compute target %q is not configured", target)
	}
	installed, err := m.models.Installed(entry.ID)
	if err != nil {
		installed, err = m.models.Pull(ctx, entry, nil)
		if err != nil {
			return nil, fmt.Errorf("install model %s: %w", entry.ID, err)
		}
	}
	adapter, err := m.registry.Select(installed.Runtime)
	if err != nil {
		return nil, err
	}
	if err = adapter.Prepare(ctx, installed, computeTarget); err != nil {
		return nil, err
	}
	gpu, err := gpuLayers(request.Options.GPULayers)
	if err != nil {
		return nil, err
	}
	session, err := adapter.Start(ctx, installed, computeTarget, backruntime.StartOptions{Host: "127.0.0.1", ContextSize: request.Options.ContextLength, GPULayers: gpu})
	if err != nil {
		return nil, err
	}
	session.ID = newID()
	m.mu.Lock()
	m.sessions[session.ID] = &managed{session, adapter, installed}
	m.persistLocked()
	m.mu.Unlock()
	go m.monitor(session.ID, session.Process)
	return clone(session), nil
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
func (m *Manager) persistLocked() {
	items := make([]*backruntime.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		items = append(items, clone(s.public))
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
	var prior []*backruntime.Session
	if json.Unmarshal(data, &prior) != nil {
		return
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
}
