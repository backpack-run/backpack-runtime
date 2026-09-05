package whispercpp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/runtimebundle"
)

type Adapter struct {
	Paths    config.Paths
	Runtimes *runtimebundle.Manager
	resolved sync.Map
}

func (*Adapter) Name() string { return "whisper.cpp" }
func (*Adapter) Supports(r models.RuntimeRequirement) bool {
	return backruntime.SameEngine(r.Engine, "whisper.cpp")
}
func (*Adapter) Capabilities() []string { return []string{"transcription"} }
func (a *Adapter) key(m *models.Installed, t compute.Target) string {
	return m.ID + "@" + m.Runtime.Version + "@" + t.Name()
}
func (a *Adapter) Prepare(ctx context.Context, m *models.Installed, t compute.Target) error {
	if m.Runtime.Environment != "native-bundle" && m.Runtime.Environment != "native-process" && m.Runtime.Environment != "" {
		return fmt.Errorf("whisper.cpp requires unexpected environment %q", m.Runtime.Environment)
	}
	if _, err := os.Stat(m.Entrypoint()); err != nil {
		return fmt.Errorf("model artifact is missing: %w", err)
	}
	if err := t.Prepare(ctx); err != nil {
		return err
	}
	installed, err := a.Runtimes.Ensure(ctx, m.Runtime, t)
	if err != nil {
		return err
	}
	a.resolved.Store(a.key(m, t), installed.Executable)
	return t.PrepareModel(ctx, m)
}
func (*Adapter) Start(context.Context, *models.Installed, compute.Target, backruntime.StartOptions) (*backruntime.Session, error) {
	return nil, fmt.Errorf("whisper.cpp transcription is job-based; use the transcription API")
}
func (*Adapter) Health(context.Context, *backruntime.Session) error {
	return fmt.Errorf("whisper.cpp jobs do not expose persistent health")
}
func (*Adapter) Stop(context.Context, *backruntime.Session) error { return nil }
func (a *Adapter) Transcribe(ctx context.Context, m *models.Installed, t compute.Target, request backruntime.TranscriptionRequest) (*backruntime.Transcription, error) {
	value, ok := a.resolved.Load(a.key(m, t))
	if !ok {
		return nil, fmt.Errorf("whisper.cpp was not prepared")
	}
	preparer, ok := t.(compute.FilePreparer)
	if !ok {
		return nil, fmt.Errorf("compute target %q cannot stage audio inputs", t.Name())
	}
	audio, err := preparer.PrepareFile(ctx, request.AudioPath)
	if err != nil {
		return nil, err
	}
	args := []string{"--model", t.ResolvePath(m.Entrypoint()), "--file", audio, "--no-prints", "--no-timestamps"}
	if request.Language != "" {
		args = append(args, "--language", request.Language)
	} else {
		args = append(args, "--language", "auto")
	}
	var stdout, stderr bytes.Buffer
	process, err := t.Execute(ctx, compute.Command{Executable: value.(string), Args: args, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = process.Stop(context.Background())
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("whisper.cpp transcription failed: %w: %s", err, tail(stderr.String(), 1000))
	}
	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, fmt.Errorf("whisper.cpp returned no transcript: %s", tail(stderr.String(), 1000))
	}
	return &backruntime.Transcription{Text: text, Language: request.Language, Model: m.ID}, nil
}
func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

var _ backruntime.Transcriber = (*Adapter)(nil)
