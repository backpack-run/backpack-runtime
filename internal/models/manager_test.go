package models

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
