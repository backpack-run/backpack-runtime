package sessions

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
)

type fakeProcess struct{ done chan struct{} }

func (f *fakeProcess) PID() int    { return 42 }
func (f *fakeProcess) Wait() error { <-f.done; return nil }
func (f *fakeProcess) Stop(context.Context) error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

type fakeAdapter struct{}

func (fakeAdapter) Name() string                                                     { return "llama.cpp" }
func (fakeAdapter) Supports(r models.RuntimeRequirement) bool                        { return r.Engine == "llama.cpp" }
func (fakeAdapter) Prepare(context.Context, *models.Installed, compute.Target) error { return nil }
func (fakeAdapter) Health(context.Context, *backruntime.Session) error               { return nil }
func (fakeAdapter) Capabilities() []string                                           { return []string{"chat"} }
func (fakeAdapter) Start(context.Context, *models.Installed, compute.Target, backruntime.StartOptions) (*backruntime.Session, error) {
	p := &fakeProcess{make(chan struct{})}
	return &backruntime.Session{ModelID: "test-model", Runtime: "llama.cpp", Compute: "local", Status: "ready", PID: 42, Process: p, CreatedAt: time.Now().UTC()}, nil
}
func (fakeAdapter) Stop(ctx context.Context, s *backruntime.Session) error {
	return s.Process.Stop(ctx)
}

func TestCreateListStopAndPersist(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	revision := "abc"
	packageID := "q4"
	dir := filepath.Join(paths.Models, "test-model", revision, packageID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := `schema_version: 1
model: {id: test-model, display_name: Test}
upstream: {repo: upstream/test, revision: abc}
packages:
  - id: q4
    format: gguf
    precision: Q4_K_M
    filename: model.gguf
    sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    size_bytes: 1
    runtime: {provider: llama.cpp}
`
	if err := os.WriteFile(filepath.Join(dir, "backpack-model.yaml"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(struct{ Repository, Revision, Package string }{"backpack-run/Test", revision, packageID})
	if err := os.WriteFile(filepath.Join(paths.Manifests, "test-model.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	c := catalog.Catalog{SchemaVersion: 1, Models: []catalog.Model{{ID: "test-model", Aliases: []string{"test"}}}}
	manager := New(c, models.NewManager(paths), backruntime.NewRegistry(fakeAdapter{}), paths)
	session, err := manager.Create(context.Background(), CreateRequest{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID == "" || len(manager.List()) != 1 {
		t.Fatal("session was not registered")
	}
	if err = manager.Stop(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := manager.Get(session.ID); got.Status != "stopped" {
		t.Fatalf("status %q", got.Status)
	}
	reloaded := New(c, models.NewManager(paths), backruntime.NewRegistry(fakeAdapter{}), paths)
	if len(reloaded.List()) != 1 {
		t.Fatal("session state was not persisted")
	}
}
