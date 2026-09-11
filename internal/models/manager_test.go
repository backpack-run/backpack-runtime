package models

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/config"
)

func TestPullVerifiesAndInstallsPackage(t *testing.T) {
	artifact := []byte("tiny gguf")
	digest := fmt.Sprintf("%x", sha256.Sum256(artifact))
	manifest := fmt.Sprintf(`schema_version: 1
model: {id: test-model, display_name: Test}
upstream: {repo: upstream/test, revision: abc}
packages:
  - id: gguf-q4-k-m
    format: gguf
    precision: Q4_K_M
    filename: model.gguf
    sha256: %s
    size_bytes: %d
    runtime: {provider: llama.cpp}
`, digest, len(artifact))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path[len(r.URL.Path)-10:] == "model.gguf" {
			_, _ = w.Write(artifact)
			return
		}
		_, _ = w.Write([]byte(manifest))
	}))
	defer server.Close()
	m := NewManager(config.NewPaths(t.TempDir()))
	m.BaseURL = server.URL
	entry := catalog.Model{ID: "test-model", Repository: "backpack-run/Test", Revision: "revision"}
	installed, err := m.Pull(context.Background(), entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(installed.Entrypoint()); err != nil {
		t.Fatal(err)
	}
	if installed.Runtime.Engine != "llama.cpp" {
		t.Fatalf("runtime %q", installed.Runtime.Engine)
	}
}

func TestInstalledModelRecordMigratesAndRejectsFutureVersion(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		paths := config.NewPaths(t.TempDir())
		if err := os.MkdirAll(paths.Manifests, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(paths.Manifests, "test.json")
		legacy := []byte(`{"repository":"org/model","revision":"abc","package":"q4"}`)
		if err := os.WriteFile(path, legacy, 0600); err != nil {
			t.Fatal(err)
		}
		_, _ = NewManager(paths).Installed("test")
		if backup, err := os.ReadFile(path + ".v0.bak"); err != nil || string(backup) != string(legacy) {
			t.Fatalf("backup=%q err=%v", backup, err)
		}
	})
	t.Run("future", func(t *testing.T) {
		paths := config.NewPaths(t.TempDir())
		if err := os.MkdirAll(paths.Manifests, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(paths.Manifests, "test.json")
		original := []byte(`{"schema_version":2,"repository":"org/model","revision":"abc","package":"q4"}`)
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		_, err := NewManager(paths).Installed("test")
		if err == nil || !strings.Contains(err.Error(), "newer than supported") {
			t.Fatalf("unexpected error: %v", err)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(original) {
			t.Fatal("future model state was modified")
		}
	})
}

func TestResolvePackageDoesNotDownloadWeights(t *testing.T) {
	artifact := []byte("large")
	digest := fmt.Sprintf("%x", sha256.Sum256(artifact))
	manifest := fmt.Sprintf(`schema_version: 1
model: {id: metadata-only, display_name: Test}
upstream: {repo: upstream/test, revision: abc}
packages:
  - id: q4
    format: gguf
    precision: Q4_K_M
    filename: large.gguf
    sha256: %s
    size_bytes: %d
    runtime: {provider: llama.cpp}
    hardware: {estimated_ram_gb: 64, recommended_ram_gb: 80}
`, digest, len(artifact))
	artifactRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "large.gguf") {
			artifactRequests++
			_, _ = w.Write(artifact)
			return
		}
		_, _ = w.Write([]byte(manifest))
	}))
	defer server.Close()
	m := NewManager(config.NewPaths(t.TempDir()))
	m.BaseURL = server.URL
	resolved, err := m.ResolvePackage(context.Background(), catalog.Model{ID: "metadata-only", Repository: "org/model", Revision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Package.Hardware.EstimatedRAMGB != 64 || artifactRequests != 0 {
		t.Fatalf("resolved=%#v artifact requests=%d", resolved.Package, artifactRequests)
	}
}
