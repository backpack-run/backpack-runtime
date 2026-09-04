package compute

import (
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
