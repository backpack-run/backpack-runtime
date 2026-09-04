package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/runtimebundle"
)

type Adapter struct {
	Paths    config.Paths
	Client   *http.Client
	Runtimes *runtimebundle.Manager
	resolved sync.Map
}

func (a *Adapter) Name() string { return "llama.cpp" }
func (a *Adapter) Supports(r models.RuntimeRequirement) bool {
	return backruntime.SameEngine(r.Engine, "llama.cpp")
}
func (a *Adapter) Capabilities() []string { return []string{"chat", "completion"} }
func (a *Adapter) executable(target compute.Target) (string, error) {
	if target.Kind() == "ssh" {
		return "llama-server", nil
	}
	if x := os.Getenv("BACKPACK_LLAMA_SERVER"); x != "" {
		if _, err := os.Stat(x); err == nil {
			return x, nil
		}
	}
	if x, err := exec.LookPath("llama-server"); err == nil {
		return x, nil
	}
	name := "llama-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidate := filepath.Join(a.Paths.Runtimes, "llama.cpp", name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("llama.cpp server is not installed; set BACKPACK_LLAMA_SERVER or place %s in %s", name, filepath.Dir(candidate))
}
func (a *Adapter) Prepare(ctx context.Context, m *models.Installed, target compute.Target) error {
	if m.Runtime.Environment != "native-process" && m.Runtime.Environment != "native-bundle" && m.Runtime.Environment != "" {
		return fmt.Errorf("llama.cpp package requires unexpected environment %q", m.Runtime.Environment)
	}
	if _, err := os.Stat(m.Entrypoint()); err != nil {
		return fmt.Errorf("model artifact is missing: %w", err)
	}
	if err := target.Prepare(ctx); err != nil {
		return err
	}
	key := m.ID + "@" + m.Runtime.Version + "@" + target.Name()
	if target.Kind() == "local" {
		if exe, err := a.executable(target); err == nil {
			a.resolved.Store(key, exe)
		} else if a.Runtimes == nil {
			return err
		}
	}
	if _, ok := a.resolved.Load(key); !ok {
		if a.Runtimes == nil {
			return fmt.Errorf("managed runtime support is unavailable")
		}
		installed, err := a.Runtimes.Ensure(ctx, m.Runtime, target)
		if err != nil {
			return err
		}
		a.resolved.Store(key, installed.Executable)
	}
	if err := target.PrepareModel(ctx, m); err != nil {
		return err
	}
	return nil
}
func (a *Adapter) Start(ctx context.Context, m *models.Installed, target compute.Target, o backruntime.StartOptions) (*backruntime.Session, error) {
	value, ok := a.resolved.Load(m.ID + "@" + m.Runtime.Version + "@" + target.Name())
	if !ok {
		return nil, fmt.Errorf("runtime was not prepared")
	}
	exe := value.(string)
	var err error
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}
	if o.Port == 0 {
		o.Port, err = freePort()
		if err != nil {
			return nil, err
		}
	}
	if o.ContextSize == 0 {
		o.ContextSize = m.Manifest.Model.ContextLength
		if o.ContextSize == 0 {
			o.ContextSize = 8192
		}
	}
	args := []string{"--model", target.ResolvePath(m.Entrypoint()), "--host", o.Host, "--port", fmt.Sprint(o.Port), "--ctx-size", fmt.Sprint(o.ContextSize), "--n-gpu-layers", fmt.Sprint(o.GPULayers), "--alias", m.ID}
	logPath := filepath.Join(a.Paths.Logs, "llama-"+m.ID+".log")
	_ = os.MkdirAll(a.Paths.Logs, 0700)
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	process, err := target.Execute(ctx, compute.Command{Executable: exe, Args: args, Stdout: log, Stderr: log})
	if err != nil {
		log.Close()
		return nil, fmt.Errorf("start llama.cpp: %w", err)
	}
	s := &backruntime.Session{ID: fmt.Sprintf("%s-%d", m.ID, process.PID()), ModelID: m.ID, Runtime: a.Name(), Compute: target.Name(), Endpoint: fmt.Sprintf("http://%s:%d", o.Host, o.Port), Status: "starting", PID: process.PID(), Process: process, CreatedAt: time.Now().UTC()}
	healthCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := a.Health(healthCtx, s); err != nil {
		stopCtx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		_ = process.Stop(stopCtx)
		return nil, fmt.Errorf("llama.cpp did not become ready: %w (see %s)", err, logPath)
	}
	s.Status = "ready"
	return s, nil
}
func (a *Adapter) Health(ctx context.Context, s *backruntime.Session) error {
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: time.Second}
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.Endpoint+"/health", nil)
		if res, err := client.Do(req); err == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode/100 == 2 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (a *Adapter) Stop(ctx context.Context, s *backruntime.Session) error {
	s.Status = "stopping"
	err := s.Process.Stop(ctx)
	s.Status = "stopped"
	return err
}
func (a *Adapter) Chat(ctx context.Context, s *backruntime.Session, prompt string) (string, error) {
	payload, _ := json.Marshal(map[string]any{"model": s.ModelID, "messages": []map[string]string{{"role": "user", "content": prompt}}, "stream": false})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint+"/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		return "", fmt.Errorf("inference returned %s: %s", res.Status, string(body))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("inference returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
