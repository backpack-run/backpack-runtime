package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

func TestClaudeInvocationIsolatesRoutingAndPreservesPassthrough(t *testing.T) {
	root := t.TempDir()
	invocation, err := ClaudeInvocation(ProviderOptions{Endpoint: "http://127.0.0.1:11434", Model: "coder", ContextTokens: 65536, ConfigDirectory: filepath.Join(root, "claude"), Executable: filepath.Join(root, "claude.exe"), Passthrough: []string{"--help"}})
	if err != nil {
		t.Fatal(err)
	}
	args := invocation.Args()
	if strings.Join(args, "|") != "--model|coder|--help" {
		t.Fatalf("unexpected args %#v", args)
	}
	env := strings.Join(invocation.Environment.Apply([]string{"ANTHROPIC_BASE_URL=https://api.anthropic.com", "ANTHROPIC_API_KEY=secret", "PATH=test"}), "\n")
	for _, required := range []string{"ANTHROPIC_BASE_URL=http://127.0.0.1:11434", "ANTHROPIC_AUTH_TOKEN=backpack-local", "CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=backpack-runtime", "CLAUDE_CONFIG_DIR=" + filepath.Join(root, "claude"), "PATH=test"} {
		if !strings.Contains(env, required) {
			t.Fatalf("missing %q in child environment %s", required, env)
		}
	}
	if strings.Contains(env, "api.anthropic.com") || strings.Contains(env, "ANTHROPIC_API_KEY=") {
		t.Fatalf("conflicting Claude credentials survived: %s", env)
	}
	if strings.Contains(invocation.Environment.String(), "backpack-local") {
		t.Fatal("diagnostics leaked the launch token")
	}
}

func TestProviderInvocationsRejectRoutingOverrides(t *testing.T) {
	root := t.TempDir()
	base := ProviderOptions{Endpoint: "http://127.0.0.1:11434", Model: "coder", ConfigDirectory: root, Executable: filepath.Join(root, "tool")}
	base.Passthrough = []string{"--model", "other"}
	if _, err := ClaudeInvocation(base); err == nil {
		t.Fatal("Claude accepted a conflicting model override")
	}
	base.CatalogPath = filepath.Join(root, "models.json")
	base.Passthrough = []string{"-c", `model_provider="openai"`}
	if _, err := CodexInvocation(base); err == nil {
		t.Fatal("Codex accepted a conflicting provider override")
	}
}

func TestCodexInvocationUsesCommandLineProviderIsolation(t *testing.T) {
	root := t.TempDir()
	options := ProviderOptions{Endpoint: "http://localhost:9876", Model: "coder", ContextTokens: 65536, ConfigDirectory: root, CatalogPath: filepath.Join(root, "models.json"), Executable: filepath.Join(root, "codex"), Passthrough: []string{"--sandbox", "read-only"}}
	invocation, err := CodexInvocation(options)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(invocation.Args(), " ")
	for _, required := range []string{`model_provider="backpack"`, `model_providers.backpack.base_url="http://localhost:9876/v1/"`, `model_providers.backpack.wire_api="responses"`, `features.apps=false`, `features.plugins=false`, `features.multi_agent=false`, "-m coder", "--sandbox read-only"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q in %s", required, joined)
		}
	}
	if strings.Contains(joined, "--dangerously") {
		t.Fatalf("launcher weakened Codex permissions: %s", joined)
	}
	environment := strings.Join(invocation.Environment.Apply([]string{"CODEX_HOME=user", "OPENAI_API_KEY=user-secret"}), "\n")
	if !strings.Contains(environment, "CODEX_HOME="+root) || !strings.Contains(environment, "OPENAI_API_KEY=backpack-local") || strings.Contains(environment, "user-secret") {
		t.Fatalf("Codex child environment was not isolated: %s", environment)
	}
	if strings.Contains(invocation.Environment.String(), "backpack-local") {
		t.Fatal("diagnostics leaked the Codex launch token")
	}
}

func TestWriteCodexModelCatalogUsesTrustedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex", "models.json")
	model := catalog.Model{ID: "coder", DisplayName: "Coder", Capabilities: []string{"chat", "code", "vision"}}
	if err := WriteCodexModelCatalog(CodexCatalogOptions{Model: model, ContextTokens: 131072, Path: path}); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Models []struct {
			Slug              string   `json:"slug"`
			ContextWindow     int      `json:"context_window"`
			InputModalities   []string `json:"input_modalities"`
			ParallelToolCalls bool     `json:"supports_parallel_tool_calls"`
		} `json:"models"`
	}
	data := mustRead(t, path)
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Models) != 1 || payload.Models[0].Slug != "coder" || payload.Models[0].ContextWindow != 131072 || len(payload.Models[0].InputModalities) != 2 || payload.Models[0].ParallelToolCalls {
		t.Fatalf("unexpected catalog %s", data)
	}
}

func TestOpenCodeInvocationUsesInlineIsolatedProvider(t *testing.T) {
	root := t.TempDir()
	invocation, err := OpenCodeInvocation(ProviderOptions{Endpoint: "http://127.0.0.1:11434", Model: "coder", ContextTokens: 65536, ConfigDirectory: root, Executable: filepath.Join(root, "opencode"), Passthrough: []string{"run", "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(invocation.Args(), " ")
	if !strings.Contains(joined, "--pure --model backpack/coder run hello") {
		t.Fatalf("unexpected OpenCode arguments: %s", joined)
	}
	environment := strings.Join(invocation.Environment.Apply([]string{"OPENCODE_CONFIG_CONTENT=user", "OPENCODE_AUTO_SHARE=true"}), "\n")
	for _, required := range []string{"OPENCODE_CONFIG_DIR=" + root, "OPENCODE_DISABLE_MODELS_FETCH=true", "OPENCODE_AUTO_SHARE=false", `"baseURL":"http://127.0.0.1:11434/v1"`} {
		if !strings.Contains(environment, required) {
			t.Fatalf("missing %q in OpenCode child environment", required)
		}
	}
	if strings.Contains(invocation.Environment.String(), "backpack-local") {
		t.Fatal("OpenCode diagnostics leaked inline provider credentials")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
