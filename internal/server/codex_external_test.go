package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/integrations"
)

// This opt-in test exercises the installed real Codex CLI while keeping its
// config and workspace disposable. Normal CI has no third-party CLI dependency.
func TestRealCodexResponsesProtocol(t *testing.T) {
	if os.Getenv("BACKPACK_TEST_CODEX") != "1" {
		t.Skip("set BACKPACK_TEST_CODEX=1 to qualify an installed Codex CLI")
	}
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed")
	}
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"BACKPACK_CODEX_PROTOCOL_OK\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":4}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer child.Close()
	backpack := httptest.NewServer((&Server{Sessions: fakeSessions{child.URL}}).Handler())
	defer backpack.Close()

	root := t.TempDir()
	model := catalog.Model{ID: "backpack-test-coder", DisplayName: "Backpack test coder", Capabilities: []string{"chat", "code"}}
	catalogPath := filepath.Join(root, "models.json")
	if err = integrations.WriteCodexModelCatalog(integrations.CodexCatalogOptions{Model: model, ContextTokens: 65536, Path: catalogPath}); err != nil {
		t.Fatal(err)
	}
	invocation, err := integrations.CodexInvocation(integrations.ProviderOptions{Endpoint: backpack.URL, Model: model.ID, ContextTokens: 65536, ConfigDirectory: root, CatalogPath: catalogPath, Executable: codex, Passthrough: []string{"exec", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config", "--sandbox", "read-only", "Reply with the exact token BACKPACK_CODEX_PROTOCOL_OK and do nothing else."}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var output bytes.Buffer
	err = integrations.Run(ctx, invocation, integrations.ProcessIO{Stdout: &output, Stderr: &output})
	if err != nil {
		t.Fatalf("real Codex protocol qualification failed: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "BACKPACK_CODEX_PROTOCOL_OK") {
		t.Fatalf("Codex did not accept the Backpack Responses stream:\n%s", output.String())
	}
}

func TestRealCodexToolRoundTrip(t *testing.T) {
	if os.Getenv("BACKPACK_TEST_CODEX") != "1" {
		t.Skip("set BACKPACK_TEST_CODEX=1 to qualify an installed Codex CLI")
	}
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("Codex CLI is not installed")
	}
	requests := 0
	child := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_backpack\",\"function\":{\"name\":\"exec_command\",\"arguments\":\"{\\\"cmd\\\":\\\"Write-Output BACKPACK_CODEX_TOOL_OK\\\"}\"}}]}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		} else {
			if !bytes.Contains(data, []byte(`"tool_call_id":"call_backpack"`)) || !bytes.Contains(data, []byte("BACKPACK_CODEX_TOOL_OK")) {
				t.Errorf("tool result did not round-trip through Codex: %s", data)
			}
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"BACKPACK_CODEX_TOOL_ROUNDTRIP_OK\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer child.Close()
	backpack := httptest.NewServer((&Server{Sessions: fakeSessions{child.URL}}).Handler())
	defer backpack.Close()

	root := t.TempDir()
	model := catalog.Model{ID: "backpack-test-coder", DisplayName: "Backpack test coder", Capabilities: []string{"chat", "code"}}
	catalogPath := filepath.Join(root, "models.json")
	if err = integrations.WriteCodexModelCatalog(integrations.CodexCatalogOptions{Model: model, ContextTokens: 65536, Path: catalogPath}); err != nil {
		t.Fatal(err)
	}
	invocation, err := integrations.CodexInvocation(integrations.ProviderOptions{Endpoint: backpack.URL, Model: model.ID, ContextTokens: 65536, ConfigDirectory: root, CatalogPath: catalogPath, Executable: codex, Passthrough: []string{"exec", "--skip-git-repo-check", "--ephemeral", "--ignore-user-config", "--sandbox", "read-only", "Run the requested command and finish."}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err = integrations.Run(ctx, invocation, integrations.ProcessIO{Stdout: &output, Stderr: &output}); err != nil {
		t.Fatalf("real Codex tool qualification failed: %v\n%s", err, output.String())
	}
	if requests != 2 || !strings.Contains(output.String(), "BACKPACK_CODEX_TOOL_ROUNDTRIP_OK") {
		t.Fatalf("Codex tool loop did not complete (requests=%d):\n%s", requests, output.String())
	}
}
