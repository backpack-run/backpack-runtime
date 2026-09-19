package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeAppConfigureReconfigureAndRestore(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path contract")
	}
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	normal := filepath.Join(local, "Claude", "claude_desktop_config.json")
	if err := os.MkdirAll(filepath.Dir(normal), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("{\n  \"unrelated\": true\n}\n")
	if err := os.WriteFile(normal, original, 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "state")
	options := ClaudeAppOptions{StateDirectory: state, Endpoint: "http://127.0.0.1:12345", APIKey: "secret-token", Model: "coder:cloud", ContextTokens: 32768}
	if err := ConfigureClaudeApp(options); err != nil {
		t.Fatal(err)
	}
	targets, _ := claudeAppTargets()
	profile, err := os.ReadFile(targets.profile)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(profile, &value); err != nil {
		t.Fatal(err)
	}
	base, _ := value["inferenceGatewayBaseUrl"].(string)
	if !strings.Contains(base, "/coder:cloud") || strings.HasSuffix(base, "/v1") {
		t.Fatalf("unexpected gateway base URL %q", base)
	}
	options.Model = "other:cloud"
	if err = ConfigureClaudeApp(options); err != nil {
		t.Fatalf("reconfigure: %v", err)
	}
	if err = RestoreClaudeApp(state); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(normal)
	if err != nil || string(restored) != string(original) {
		t.Fatalf("restore err=%v data=%q", err, restored)
	}
	if _, err = os.Stat(targets.profile); !os.IsNotExist(err) {
		t.Fatalf("new profile survived restore: %v", err)
	}
}

func TestResolveContextWindowUsesQualifiedMaximum(t *testing.T) {
	if got := ResolveContextWindow(0, 32768, RecommendedAgentContext); got != 32768 {
		t.Fatalf("qualified maximum = %d", got)
	}
	if got := ResolveContextWindow(8192, 32768, RecommendedAgentContext); got != 8192 {
		t.Fatalf("explicit context = %d", got)
	}
	if got := ResolveContextWindow(0, 0, RecommendedAgentContext); got != RecommendedAgentContext {
		t.Fatalf("fallback context = %d", got)
	}
}
