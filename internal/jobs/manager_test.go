package jobs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/artifacts"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
)

type fakeRunner struct{ started chan struct{} }

func (f fakeRunner) Capability() string { return "image-generation" }
func (f fakeRunner) Runtime() string    { return "fake-image" }
func (f fakeRunner) Run(ctx context.Context, _ string, _ CreateRequest, report Reporter) ([]artifacts.Artifact, error) {
	close(f.started)
	report(Running, Progress{Step: 1, Total: 2, Message: "step 1"})
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestJobLifecycleAndCancellation(t *testing.T) {
	c := catalog.Catalog{Models: []catalog.Model{{ID: "image", Aliases: []string{"img"}, Capabilities: []string{"image-generation"}}}}
	runner := fakeRunner{started: make(chan struct{})}
	m := New(c, config.NewPaths(t.TempDir()), nil, runner)
	job, err := m.Create(context.Background(), CreateRequest{Model: "img", Capability: "image-generation", Input: Input{Prompt: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}
	if err = m.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, getErr := m.Get(job.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current.Status == Cancelled {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job was not cancelled")
}

func TestRequestValidationAndUnavailableRunner(t *testing.T) {
	c := catalog.Catalog{Models: []catalog.Model{{ID: "video", Capabilities: []string{"video-generation"}}}}
	m := New(c, config.NewPaths(t.TempDir()), nil)
	_, err := m.Create(context.Background(), CreateRequest{Model: "video", Capability: "video-generation", Input: Input{Prompt: "test"}, Options: Options{Frames: 1001}})
	if err == nil {
		t.Fatal("unsafe frame count accepted")
	}
	_, err = m.Create(context.Background(), CreateRequest{Model: "video", Capability: "video-generation", Input: Input{Prompt: "test"}})
	if err == nil || !strings.Contains(err.Error(), "no execution-validated runtime adapter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompletedArtifactSurvivesManagerRestartWithoutPublicPath(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	jobID := "job-restart"
	artifactID := "image"
	directory, err := (artifacts.Manager{Root: paths.Outputs}).JobDirectory(jobID)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "result.png")
	if err = os.WriteFile(output, []byte("image bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	prior := []*Job{{ID: jobID, Model: "image", Capability: "image-generation", Status: Completed, CreatedAt: time.Now().UTC(), Artifacts: []artifacts.Artifact{{ID: artifactID, Filename: "result.png", MediaType: "image/png"}}}}
	data, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(paths.State, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(paths.State, "jobs.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	m := New(catalog.Catalog{}, paths, nil)
	if m.StateError() != nil {
		t.Fatal(m.StateError())
	}
	if _, err = os.Stat(filepath.Join(paths.State, "jobs.json.v0.bak")); err != nil {
		t.Fatalf("legacy job state was not backed up: %v", err)
	}
	resolved, err := m.ArtifactPath(jobID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != output {
		t.Fatalf("resolved artifact %q, want %q", resolved, output)
	}
	public, err := json.Marshal(m.List()[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), output) || strings.Contains(string(public), `"path"`) {
		t.Fatalf("public job leaked artifact path: %s", public)
	}
}

func TestFutureJobStateIsRefusedAndPreserved(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := os.MkdirAll(paths.State, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(paths.State, "jobs.json")
	original := []byte(`{"schema_version":2,"jobs":[]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	m := New(catalog.Catalog{}, paths, nil)
	if m.StateError() == nil || !strings.Contains(m.StateError().Error(), "newer than supported") {
		t.Fatalf("unexpected state error: %v", m.StateError())
	}
	if _, err := m.Create(context.Background(), CreateRequest{}); err == nil {
		t.Fatal("manager accepted a write with future state")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("future job state was modified")
	}
}
