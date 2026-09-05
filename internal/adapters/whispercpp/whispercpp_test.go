package whispercpp

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
)

type recordingTarget struct {
	command compute.Command
}

func (*recordingTarget) Name() string { return "local" }
func (*recordingTarget) Kind() string { return "local" }
func (*recordingTarget) Inspect(context.Context) (compute.Hardware, error) {
	return compute.Hardware{}, nil
}
func (*recordingTarget) Prepare(context.Context) error                         { return nil }
func (*recordingTarget) PrepareModel(context.Context, *models.Installed) error { return nil }
func (*recordingTarget) ResolvePath(path string) string                        { return path }
func (t *recordingTarget) PrepareFile(_ context.Context, path string) (string, error) {
	return path, nil
}
func (t *recordingTarget) Execute(_ context.Context, command compute.Command) (compute.Process, error) {
	t.command = command
	_, _ = io.WriteString(command.Stdout, " A small test transcript. \n")
	return completedProcess{}, nil
}

type completedProcess struct{}

func (completedProcess) PID() int                   { return 42 }
func (completedProcess) Wait() error                { return nil }
func (completedProcess) Stop(context.Context) error { return nil }

func TestTranscribeBuildsNativeJob(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.bin")
	audioPath := filepath.Join(dir, "speech.wav")
	for _, path := range []string{modelPath, audioPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	model := &models.Installed{ID: "whisper", Directory: dir, Package: models.Package{Filename: "model.bin"}, Runtime: models.RuntimeRequirement{Engine: "whisper.cpp", Version: "revision"}}
	target := &recordingTarget{}
	adapter := &Adapter{}
	adapter.resolved.Store(adapter.key(model, target), "whisper-cli.exe")
	result, err := adapter.Transcribe(context.Background(), model, target, backruntime.TranscriptionRequest{AudioPath: audioPath, Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "A small test transcript." || result.Model != "whisper" {
		t.Fatalf("result %#v", result)
	}
	want := []string{"--model", modelPath, "--file", audioPath, "--no-prints", "--no-timestamps", "--language", "en"}
	if !reflect.DeepEqual(target.command.Args, want) {
		t.Fatalf("args %#v, want %#v", target.command.Args, want)
	}
}
