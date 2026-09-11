package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/pkg/client"
)

type State struct {
	SchemaVersion int       `json:"schema_version"`
	PID           int       `json:"pid"`
	Endpoint      string    `json:"endpoint"`
	StartedAt     time.Time `json:"started_at"`
	Version       string    `json:"version"`
}

func statePath(p config.Paths) string { return filepath.Join(p.State, "runtime.json") }
func lockPath(p config.Paths) string  { return filepath.Join(p.State, "startup.lock") }
func Read(p config.Paths) (State, error) {
	var s State
	data, err := os.ReadFile(statePath(p))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	if err == nil && s.SchemaVersion > 1 {
		return State{}, fmt.Errorf("runtime state schema_version %d is newer than supported 1; upgrade Backpack", s.SchemaVersion)
	}
	return s, err
}
func Write(p config.Paths, s State) error {
	if err := os.MkdirAll(p.State, 0700); err != nil {
		return err
	}
	s.SchemaVersion = 1
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := statePath(p) + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	_ = os.Remove(statePath(p))
	return os.Rename(tmp, statePath(p))
}
func ClearIfOwned(p config.Paths, pid int) {
	state, err := Read(p)
	if err == nil && state.PID == pid {
		_ = os.Remove(statePath(p))
	}
}

func Ensure(ctx context.Context, p config.Paths, version string) (*client.Client, error) {
	if s, err := Read(p); err == nil && s.Endpoint != "" {
		c := client.New(s.Endpoint)
		check, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
		err = c.Health(check)
		cancel()
		if err == nil {
			return c, nil
		}
	}
	if err := p.Ensure(); err != nil {
		return nil, err
	}
	lock, err := acquire(ctx, p)
	if err != nil {
		return nil, err
	}
	defer func() { lock.Close(); _ = os.Remove(lockPath(p)) }()
	if s, err := Read(p); err == nil && s.Endpoint != "" {
		c := client.New(s.Endpoint)
		check, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
		err = c.Health(check)
		cancel()
		if err == nil {
			return c, nil
		}
	}
	endpoint, err := endpoint()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(p.Logs, "runtime-service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "_daemon", "--address", endpoint)
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	configureBackground(cmd)
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start runtime service: %w", err)
	}
	state := State{PID: cmd.Process.Pid, Endpoint: "http://" + endpoint, StartedAt: time.Now().UTC(), Version: version}
	if err = Write(p, state); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	_ = cmd.Process.Release()
	c := client.New(state.Endpoint)
	wait, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = c.WaitHealthy(wait); err != nil {
		return nil, fmt.Errorf("runtime service did not become healthy: %w (see %s)", err, filepath.Join(p.Logs, "runtime-service.log"))
	}
	return c, nil
}
func endpoint() (string, error) {
	preferred := "127.0.0.1:11434"
	if l, err := net.Listen("tcp", preferred); err == nil {
		l.Close()
		return preferred, nil
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := l.Addr().String()
	l.Close()
	return address, nil
}
func acquire(ctx context.Context, p config.Paths) (*os.File, error) {
	path := lockPath(p)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			return f, nil
		}
		info, statErr := os.Stat(path)
		if statErr == nil && time.Since(info.ModTime()) > 20*time.Second {
			_ = os.Remove(path)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for runtime startup lock: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
