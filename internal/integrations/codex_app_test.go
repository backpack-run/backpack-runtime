package integrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

func TestCodexAppCombinesBackpackAndNativeModelsWithoutReadingAuth(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".codex", "config.toml")
	stateDirectory := filepath.Join(root, "backpack", "codex-app")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	native := `{"client_version":"test","models":[{"slug":"gpt-account-model","display_name":"Account model","priority":7,"supported_in_api":true}]}`
	if err := os.WriteFile(filepath.Join(filepath.Dir(configPath), "models_cache.json"), []byte(native), 0600); err != nil {
		t.Fatal(err)
	}
	// A sentinel proves configuration has no reason to parse the auth file.
	if err := os.WriteFile(filepath.Join(filepath.Dir(configPath), "auth.json"), []byte("not-json-and-private"), 0600); err != nil {
		t.Fatal(err)
	}
	options := CodexAppOptions{ConfigPath: configPath, StateDirectory: stateDirectory, Endpoint: "http://127.0.0.1:11434", APIKey: "private-route-token", Model: catalog.Model{ID: "coder:cloud", DisplayName: "Coder", Capabilities: []string{"code"}}, ContextTokens: 65536}
	if err := ConfigureCodexApp(options); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Models []struct {
			Slug           string `json:"slug"`
			SupportedInAPI bool   `json:"supported_in_api"`
		} `json:"models"`
	}
	if err := json.Unmarshal(mustRead(t, filepath.Join(stateDirectory, "models.json")), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 2 || result.Models[0].Slug != "coder:cloud" || !result.Models[0].SupportedInAPI || result.Models[1].Slug != "gpt-account-model" || result.Models[1].SupportedInAPI {
		t.Fatalf("combined catalog = %#v", result.Models)
	}
}

func TestCodexAppConfigureAndRestorePreservesOriginalConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".codex", "config.toml")
	stateDirectory := filepath.Join(root, "backpack", "codex-app")
	original := "model = \"openai-model\"\nmodel_provider = \"openai\"\nprofile = \"usual\"\nopenai_base_url = \"https://api.openai.com/v1\"\napproval_policy = \"on-request\"\n\n[features]\napps = true\n"
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	options := CodexAppOptions{ConfigPath: configPath, StateDirectory: stateDirectory, Endpoint: "http://127.0.0.1:11434", APIKey: "private-route-token", Model: catalog.Model{ID: "coder:cloud", DisplayName: "Coder", Capabilities: []string{"code"}}, ContextTokens: 65536}
	if err := ConfigureCodexApp(options); err != nil {
		t.Fatal(err)
	}
	configured := string(mustRead(t, configPath))
	for _, required := range []string{`model = "coder:cloud"`, `model_catalog_json = "`, `openai_base_url = "http://127.0.0.1:11434/api/backpack/v1/integrations/codex-app/private-route-token/v1"`, `approval_policy = "on-request"`, "[features]", "apps = true"} {
		if !strings.Contains(configured, required) {
			t.Fatalf("configured file omitted %q:\n%s", required, configured)
		}
	}
	if strings.Contains(configured, "model_provider") || strings.Contains(configured, "profile =") {
		t.Fatalf("configured file retained incompatible provider/profile selection:\n%s", configured)
	}
	if err := RestoreCodexApp(configPath, stateDirectory); err != nil {
		t.Fatal(err)
	}
	if restored := string(mustRead(t, configPath)); restored != original {
		t.Fatalf("restore changed original config:\n%s", restored)
	}
	if _, err := os.Stat(filepath.Join(stateDirectory, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("restore state was not removed: %v", err)
	}
}

func TestCodexAppRestoreRefusesConfigDrift(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".codex", "config.toml")
	stateDirectory := filepath.Join(root, "backpack", "codex-app")
	if err := ConfigureCodexApp(CodexAppOptions{ConfigPath: configPath, StateDirectory: stateDirectory, Endpoint: "http://localhost:11434", APIKey: "route-token", Model: catalog.Model{ID: "coder", DisplayName: "Coder", Capabilities: []string{"code"}}, ContextTokens: 65536}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, append(mustRead(t, configPath), []byte("# user edit\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreCodexApp(configPath, stateDirectory); err == nil || !strings.Contains(err.Error(), "refusing destructive restore") {
		t.Fatalf("expected safe drift refusal, got %v", err)
	}
}

func TestCodexAppExplicitReconfigurePreservesInterveningChanges(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".codex", "config.toml")
	stateDirectory := filepath.Join(root, "backpack", "codex-app")
	options := CodexAppOptions{ConfigPath: configPath, StateDirectory: stateDirectory, Endpoint: "http://localhost:11434", APIKey: "route-token", Model: catalog.Model{ID: "coder", DisplayName: "Coder", Capabilities: []string{"code"}}, ContextTokens: 65536}
	if err := ConfigureCodexApp(options); err != nil {
		t.Fatal(err)
	}
	changed := append(mustRead(t, configPath), []byte("\n[desktop]\nnew_setting = true\n")...)
	if err := os.WriteFile(configPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	options.APIKey = "fresh-route-token"
	if err := ConfigureCodexApp(options); err != nil {
		t.Fatal(err)
	}
	if configured := string(mustRead(t, configPath)); !strings.Contains(configured, "fresh-route-token") || !strings.Contains(configured, "new_setting = true") {
		t.Fatalf("reconfiguration lost managed or intervening settings:\n%s", configured)
	}
	if err := RestoreCodexApp(configPath, stateDirectory); err != nil {
		t.Fatal(err)
	}
	restored := string(mustRead(t, configPath))
	if !strings.Contains(restored, "new_setting = true") || strings.Contains(restored, "fresh-route-token") || strings.Contains(restored, "model_catalog_json") {
		t.Fatalf("rebased restore = %s", restored)
	}
}

func TestCodexAppRejectsSymlinkedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.toml")
	if err := os.Symlink(target, configPath); err != nil {
		t.Skip(err)
	}
	err := ConfigureCodexApp(CodexAppOptions{ConfigPath: configPath, StateDirectory: filepath.Join(root, "state"), Endpoint: "http://127.0.0.1:11434", APIKey: "route-token", Model: catalog.Model{ID: "coder", DisplayName: "Coder"}, ContextTokens: 65536})
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
}
