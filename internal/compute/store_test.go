package compute

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/config"
	"testing"
)

func TestTargetStoreRoundTrip(t *testing.T) {
	store := NewTargetStore(config.NewPaths(t.TempDir()))
	target := SSHConfig{ID: "gpu-1", Host: "gpu.example.org", User: "alice", Port: 22}
	if err := store.Put(target); err != nil {
		t.Fatal(err)
	}
	items, err := store.List()
	if err != nil || len(items) != 1 || items[0].Host != target.Host {
		t.Fatalf("round trip: %#v %v", items, err)
	}
	if err = store.Remove("gpu-1"); err != nil {
		t.Fatal(err)
	}
	items, _ = store.List()
	if len(items) != 0 {
		t.Fatal("target was not removed")
	}
}

func TestTargetStoreMigratesLegacyArrayWithBackup(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := os.MkdirAll(paths.Config, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(paths.Config, "compute-targets.json")
	legacy := []byte(`[{"id":"gpu-1","host":"gpu.example.org","user":"alice","port":22}]`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	items, err := NewTargetStore(paths).List()
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if backup, err := os.ReadFile(path + ".v0.bak"); err != nil || string(backup) != string(legacy) {
		t.Fatalf("backup=%q err=%v", backup, err)
	}
}

func TestTargetStoreRejectsFutureVersion(t *testing.T) {
	paths := config.NewPaths(t.TempDir())
	if err := os.MkdirAll(paths.Config, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(paths.Config, "compute-targets.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":2,"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewTargetStore(paths).List()
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}
