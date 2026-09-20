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
	// Claude adds its own settings after startup. Those additions must not make
	// a subsequent Backpack launch fail or be destroyed during restore.
	thirdParty, err := os.ReadFile(targets.thirdPartyConfig)
	if err != nil {
		t.Fatal(err)
	}
	var thirdPartyValue map[string]any
	if err = json.Unmarshal(thirdParty, &thirdPartyValue); err != nil {
		t.Fatal(err)
	}
	thirdPartyValue["preferences"] = map[string]any{"theme": "dark"}
	thirdPartyValue["coworkUserFilesPath"] = `C:\Users\test\Claude`
	thirdParty, _ = json.MarshalIndent(thirdPartyValue, "", "  ")
	if err = os.WriteFile(targets.thirdPartyConfig, append(thirdParty, '\n'), 0600); err != nil {
		t.Fatal(err)
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
	thirdParty, err = os.ReadFile(targets.thirdPartyConfig)
	if err != nil {
		t.Fatalf("Claude-owned settings file was removed: %v", err)
	}
	thirdPartyValue = map[string]any{}
	if err = json.Unmarshal(thirdParty, &thirdPartyValue); err != nil {
		t.Fatal(err)
	}
	if _, exists := thirdPartyValue["deploymentMode"]; exists || thirdPartyValue["coworkUserFilesPath"] == nil || thirdPartyValue["preferences"] == nil {
		t.Fatalf("restore did not preserve Claude-owned settings: %#v", thirdPartyValue)
	}
}

func TestClaudeAppRejectsManagedModeDrift(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path contract")
	}
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	state := filepath.Join(t.TempDir(), "state")
	options := ClaudeAppOptions{StateDirectory: state, Endpoint: "http://127.0.0.1:12345", APIKey: "secret-token", Model: "coder:cloud", ContextTokens: 32768}
	if err := ConfigureClaudeApp(options); err != nil {
		t.Fatal(err)
	}
	targets, _ := claudeAppTargets()
	if err := os.WriteFile(targets.thirdPartyConfig, []byte("{\"deploymentMode\":\"consumer\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureClaudeApp(options); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("managed mode drift was accepted: %v", err)
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
