package daemon

import (
	"context"
	"encoding/json"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/pkg/client"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestReplaceOutdatedDaemonRequiresIdleSessionsAndStopsCleanly(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/backpack/v1/version":
			_, _ = w.Write([]byte(`{"version":"old"}`))
		case "/api/backpack/v1/sessions":
			_, _ = w.Write([]byte(`{"data":[{"id":"historical","status":"stopped"},{"id":"failed","status":"failed"}]}`))
		case "/api/backpack/v1/shutdown":
			w.WriteHeader(http.StatusAccepted)
			go func() {
				time.Sleep(10 * time.Millisecond)
				server.Close()
			}()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	state := State{PID: os.Getpid(), Endpoint: server.URL, Version: "old"}
	if err := Write(paths, state); err != nil {
		t.Fatal(err)
	}
	if err := replaceOutdated(context.Background(), paths, state, client.New(server.URL), "new"); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(paths); !os.IsNotExist(err) {
		t.Fatalf("outdated daemon state was not cleared: %v", err)
	}
}

func TestReplaceOutdatedDaemonRefusesActiveSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/backpack/v1/version":
			_, _ = w.Write([]byte(`{"version":"old"}`))
		case "/api/backpack/v1/sessions":
			_, _ = w.Write([]byte(`{"data":[{"id":"active","status":"ready"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	err := replaceOutdated(context.Background(), config.NewPaths(t.TempDir()), State{Endpoint: server.URL, Version: "old"}, client.New(server.URL), "new")
	if err == nil || !strings.Contains(err.Error(), "active session") {
		t.Fatalf("active sessions did not block daemon replacement: %v", err)
	}
}
