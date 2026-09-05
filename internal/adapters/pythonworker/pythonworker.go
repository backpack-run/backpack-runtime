package pythonworker

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"github.com/backpack-run/backpack-runtime/internal/pythonruntime"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
)

type Adapter struct {
	Engine       string
	Provides     []string
	Paths        config.Paths
	Environments *pythonruntime.Manager
	HTTP         *http.Client
	resolved     sync.Map
}

func (a *Adapter) Name() string { return a.Engine }
func (a *Adapter) Supports(requirement models.RuntimeRequirement) bool {
	return backruntime.SameEngine(requirement.Engine, a.Engine)
}
func (a *Adapter) Capabilities() []string { return append([]string(nil), a.Provides...) }
func (a *Adapter) key(model *models.Installed, target compute.Target) string {
	return model.ID + "@" + model.Runtime.Version + "@" + target.Name()
}
func (a *Adapter) Prepare(ctx context.Context, model *models.Installed, target compute.Target) error {
	if model.Runtime.Environment != "isolated-python" {
		return fmt.Errorf("%s requires isolated-python, not %q", a.Engine, model.Runtime.Environment)
	}
	if err := target.Prepare(ctx); err != nil {
		return err
	}
	environment, err := a.Environments.Ensure(ctx, model.Runtime, target)
	if err != nil {
		return err
	}
	if err = target.PrepareModel(ctx, model); err != nil {
		return err
	}
	a.resolved.Store(a.key(model, target), environment)
	return nil
}
func (a *Adapter) Start(ctx context.Context, model *models.Installed, target compute.Target, options backruntime.StartOptions) (*backruntime.Session, error) {
	value, ok := a.resolved.Load(a.key(model, target))
	if !ok {
		return nil, fmt.Errorf("%s environment was not prepared", a.Engine)
	}
	environment := value.(*pythonruntime.Environment)
	if options.Host == "" {
		options.Host = "127.0.0.1"
	}
	if options.Port == 0 {
		var err error
		options.Port, err = freePort()
		if err != nil {
			return nil, err
		}
	}
	if options.Host != "127.0.0.1" {
		return nil, fmt.Errorf("Python workers must bind to 127.0.0.1")
	}
	if err := os.MkdirAll(a.Paths.Logs, 0700); err != nil {
		return nil, err
	}
	logPath := filepath.Join(a.Paths.Logs, "python-"+model.ID+".log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--no-project", "--python", environment.Python, environment.Worker, "--host", options.Host, "--port", fmt.Sprint(options.Port)}
	process, err := target.Execute(ctx, compute.Command{Executable: environment.UV, Args: args, Env: []string{"UV_CACHE_DIR=" + filepath.Join(a.Paths.Cache, "uv"), "UV_PYTHON_INSTALL_DIR=" + filepath.Join(a.Paths.Runtimes, "python", "distributions"), "UV_NO_CONFIG=1"}, Stdout: log, Stderr: log})
	if err != nil {
		_ = log.Close()
		return nil, fmt.Errorf("start %s worker: %w", a.Engine, err)
	}
	session := &backruntime.Session{ModelID: model.ID, Runtime: a.Engine, Compute: target.Name(), Endpoint: fmt.Sprintf("http://%s:%d", options.Host, options.Port), Status: "starting", PID: process.PID(), Process: process, CreatedAt: time.Now().UTC()}
	ready, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := pythonruntime.NewWorkerClient(session.Endpoint)
	client.HTTP = a.client()
	if err = client.WaitReady(ready); err == nil {
		err = client.Load(ready, target.ResolvePath(model.Directory))
	}
	if err != nil {
		stop, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = process.Stop(stop)
		return nil, fmt.Errorf("%s worker did not become ready: %w (see %s)", a.Engine, err, logPath)
	}
	session.Status = "ready"
	return session, nil
}
func (a *Adapter) Health(ctx context.Context, session *backruntime.Session) error {
	client := pythonruntime.NewWorkerClient(session.Endpoint)
	client.HTTP = a.client()
	return client.WaitReady(ctx)
}
func (a *Adapter) Stop(ctx context.Context, session *backruntime.Session) error {
	client := pythonruntime.NewWorkerClient(session.Endpoint)
	client.HTTP = a.client()
	_ = client.Unload(ctx)
	_ = client.Shutdown(ctx)
	done := make(chan error, 1)
	go func() { done <- session.Process.Wait() }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return session.Process.Stop(ctx)
	}
}
func (a *Adapter) TranscribeSession(ctx context.Context, session *backruntime.Session, request backruntime.TranscriptionRequest) (*backruntime.Transcription, error) {
	if a.Engine != "qwen-asr" {
		return nil, fmt.Errorf("runtime %s does not support transcription", a.Engine)
	}
	var response struct {
		Text, Language string
	}
	client := pythonruntime.NewWorkerClient(session.Endpoint)
	client.HTTP = a.client()
	if err := client.Infer(ctx, map[string]any{"audio_path": request.AudioPath, "language": request.Language}, &response); err != nil {
		return nil, err
	}
	return &backruntime.Transcription{Text: strings.TrimSpace(response.Text), Language: response.Language, Model: session.ModelID}, nil
}
func (a *Adapter) SynthesizeSession(ctx context.Context, session *backruntime.Session, request backruntime.SpeechRequest) (*backruntime.Speech, error) {
	if a.Engine != "kokoro" {
		return nil, fmt.Errorf("runtime %s does not support speech synthesis", a.Engine)
	}
	var response struct {
		Audio      string `json:"audio_base64"`
		Format     string `json:"format"`
		SampleRate int    `json:"sample_rate"`
	}
	client := pythonruntime.NewWorkerClient(session.Endpoint)
	client.HTTP = a.client()
	if err := client.Infer(ctx, map[string]any{"input": request.Input, "voice": request.Voice, "speed": request.Speed, "format": request.Format}, &response); err != nil {
		return nil, err
	}
	audio, err := base64.StdEncoding.DecodeString(response.Audio)
	if err != nil {
		return nil, fmt.Errorf("decode worker audio: %w", err)
	}
	return &backruntime.Speech{Audio: audio, Format: response.Format, SampleRate: response.SampleRate, Model: session.ModelID}, nil
}
func (a *Adapter) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 0}
}
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

var _ backruntime.SessionTranscriber = (*Adapter)(nil)
var _ backruntime.SessionSynthesizer = (*Adapter)(nil)
