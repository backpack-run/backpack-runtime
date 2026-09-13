package daemon

import (
	"context"
	"encoding/json"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"os"
	"testing"
	"time"
)

func TestRuntimeStateVersioningAndLegacyRead(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := Write(paths, State{PID: 42, Endpoint: "http://127.0.0.1:1234"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(statePath(paths))
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err = json.Unmarshal(data, &persisted); err != nil || persisted["schema_version"] != float64(1) {
		t.Fatalf("runtime state is not versioned: %s (%v)", data, err)
	}
	legacy := []byte(`{"pid":7,"endpoint":"http://127.0.0.1:4321"}`)
	if err = os.WriteFile(statePath(paths), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if state, readErr := Read(paths); readErr != nil || state.PID != 7 {
		t.Fatalf("legacy runtime state read failed: %#v %v", state, readErr)
	}
	if err = os.WriteFile(statePath(paths), []byte(`{"schema_version":2,"pid":7}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Read(paths); err == nil {
		t.Fatal("future runtime state version was accepted")
	}
}

func TestStartupLockIsExclusiveAndRecoversStaleFile(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	first, err := acquire(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if second, err := acquire(ctx, paths); err == nil {
		second.Close()
		t.Fatal("second startup acquired lock")
	}
	first.Close()
	old := time.Now().Add(-time.Minute)
	if err = os.Chtimes(lockPath(paths), old, old); err != nil {
		t.Fatal(err)
	}
	recovered, err := acquire(context.Background(), paths)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Close()
}

func TestDaemonAPIKeyIsRandomAndPersisted(t *testing.T) {
	first, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 40 || first == second {
		t.Fatal("daemon API keys are missing entropy")
	}
	if !ValidAPIKey(first) || ValidAPIKey("weak") {
		t.Fatal("daemon API key validation is incorrect")
	}
	paths := config.NewPaths(t.TempDir())
	if err = Write(paths, State{PID: 42, Endpoint: "http://127.0.0.1:1234", APIKey: first}); err != nil {
		t.Fatal(err)
	}
	state, err := Read(paths)
	if err != nil || state.APIKey != first {
		t.Fatalf("daemon API key was not persisted: %#v %v", state, err)
	}
}
