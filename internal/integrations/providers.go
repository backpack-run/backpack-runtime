package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

const RecommendedAgentContext = 64 * 1024

func Builtins() (*Registry, error) {
	return NewRegistry(
		Descriptor{ID: "claude", DisplayName: "Claude Code", ExecutableCandidates: []string{"claude"}, RequiredModelCapability: "code", RecommendedContextTokens: RecommendedAgentContext},
		Descriptor{ID: "codex", DisplayName: "Codex CLI", ExecutableCandidates: []string{"codex"}, RequiredModelCapability: "code", RecommendedContextTokens: RecommendedAgentContext},
		Descriptor{ID: "opencode", DisplayName: "OpenCode", ExecutableCandidates: []string{"opencode"}, RequiredModelCapability: "code", RecommendedContextTokens: RecommendedAgentContext},
	)
}

type ProviderOptions struct {
	Endpoint        string
	Model           string
	ContextTokens   int
	ConfigDirectory string
	Executable      string
	Passthrough     []string
	CatalogPath     string
}

func ClaudeInvocation(options ProviderOptions) (Invocation, error) {
	if err := validateProviderOptions(options); err != nil {
		return Invocation{}, err
	}
	if err := rejectManagedArguments("claude", options.Passthrough, "-m", "--model", "--settings"); err != nil {
		return Invocation{}, err
	}
	managed, _ := NewArguments("--model", options.Model)
	passthrough, err := NewArguments(options.Passthrough...)
	if err != nil {
		return Invocation{}, err
	}
	values := []Variable{}
	add := func(variable Variable, err error) error {
		if err != nil {
			return err
		}
		values = append(values, variable)
		return nil
	}
	for _, item := range []struct {
		name, value string
		sensitive   bool
	}{
		{"ANTHROPIC_BASE_URL", strings.TrimRight(options.Endpoint, "/"), false},
		{"ANTHROPIC_AUTH_TOKEN", "backpack-local", true},
		{"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST", "backpack-runtime", false},
		{"CLAUDE_CONFIG_DIR", options.ConfigDirectory, false},
		{"ANTHROPIC_MODEL", options.Model, false},
		{"ANTHROPIC_DEFAULT_MODEL", options.Model, false},
		{"ANTHROPIC_DEFAULT_OPUS_MODEL", options.Model, false},
		{"ANTHROPIC_DEFAULT_SONNET_MODEL", options.Model, false},
		{"ANTHROPIC_DEFAULT_HAIKU_MODEL", options.Model, false},
		{"ANTHROPIC_DEFAULT_FABLE_MODEL", options.Model, false},
		{"CLAUDE_CODE_SUBAGENT_MODEL", options.Model, false},
		{"CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS", "1", false},
		{"ENABLE_TOOL_SEARCH", "false", false},
		{"CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING", "1", false},
		{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1", false},
		{"DISABLE_TELEMETRY", "1", false},
		{"DISABLE_ERROR_REPORTING", "1", false},
	} {
		var err error
		if item.sensitive {
			err = add(Secret(item.name, item.value))
		} else {
			err = add(Set(item.name, item.value))
		}
		if err != nil {
			return Invocation{}, err
		}
	}
	if err = add(Unset("ANTHROPIC_API_KEY")); err != nil {
		return Invocation{}, err
	}
	if options.ContextTokens > 0 {
		if err = add(Set("CLAUDE_CODE_MAX_CONTEXT_TOKENS", strconv.Itoa(options.ContextTokens))); err != nil {
			return Invocation{}, err
		}
	}
	environment, err := NewEnvironmentOverlay(values...)
	if err != nil {
		return Invocation{}, err
	}
	invocation := Invocation{Executable: options.Executable, ManagedArguments: managed, PassthroughArguments: passthrough, Environment: environment, IsolatedConfigDirectory: options.ConfigDirectory}
	return invocation, invocation.Validate()
}

func CodexInvocation(options ProviderOptions) (Invocation, error) {
	if err := validateProviderOptions(options); err != nil {
		return Invocation{}, err
	}
	if options.CatalogPath == "" || !filepath.IsAbs(options.CatalogPath) {
		return Invocation{}, fmt.Errorf("Codex model catalog path must be absolute")
	}
	if err := rejectManagedArguments("codex", options.Passthrough, "-m", "--model", "-p", "--profile", "-c", "--config", "--oss", "--local-provider"); err != nil {
		return Invocation{}, err
	}
	baseURL := strings.TrimRight(options.Endpoint, "/") + "/v1/"
	managedValues := []string{
		"-c", `model_provider="backpack"`,
		"-c", fmt.Sprintf("model_providers.backpack.name=%q", "Backpack"),
		"-c", fmt.Sprintf("model_providers.backpack.base_url=%q", baseURL),
		"-c", `model_providers.backpack.wire_api="responses"`,
		"-c", fmt.Sprintf("model_catalog_json=%q", options.CatalogPath),
		"-c", `features.apps=false`,
		"-c", `features.plugins=false`,
		"-c", `features.multi_agent=false`,
		"-m", options.Model,
	}
	managed, _ := NewArguments(managedValues...)
	passthrough, err := NewArguments(options.Passthrough...)
	if err != nil {
		return Invocation{}, err
	}
	apiKey, _ := Secret("OPENAI_API_KEY", "backpack-local")
	configHome, _ := Set("CODEX_HOME", options.ConfigDirectory)
	environment, _ := NewEnvironmentOverlay(apiKey, configHome)
	invocation := Invocation{Executable: options.Executable, ManagedArguments: managed, PassthroughArguments: passthrough, Environment: environment, IsolatedConfigDirectory: options.ConfigDirectory}
	return invocation, invocation.Validate()
}

func OpenCodeInvocation(options ProviderOptions) (Invocation, error) {
	if err := validateProviderOptions(options); err != nil {
		return Invocation{}, err
	}
	if err := rejectManagedArguments("opencode", options.Passthrough, "-m", "--model"); err != nil {
		return Invocation{}, err
	}
	payload := map[string]any{
		"$schema":     "https://opencode.ai/config.json",
		"model":       "backpack/" + options.Model,
		"small_model": "backpack/" + options.Model,
		"autoupdate":  false,
		"provider": map[string]any{"backpack": map[string]any{
			"npm":  "@ai-sdk/openai-compatible",
			"name": "Backpack Runtime",
			"options": map[string]any{
				"baseURL": strings.TrimRight(options.Endpoint, "/") + "/v1",
				"apiKey":  "backpack-local",
			},
			"models": map[string]any{options.Model: map[string]any{
				"name":  options.Model,
				"limit": map[string]int{"context": options.ContextTokens, "output": min(options.ContextTokens, 16384)},
			}},
		}},
	}
	configuration, err := json.Marshal(payload)
	if err != nil {
		return Invocation{}, err
	}
	managed, _ := NewArguments("--pure", "--model", "backpack/"+options.Model)
	passthrough, err := NewArguments(options.Passthrough...)
	if err != nil {
		return Invocation{}, err
	}
	values := []Variable{}
	for _, pair := range []struct{ name, value string }{
		{"OPENCODE_CONFIG_DIR", options.ConfigDirectory},
		{"OPENCODE_DISABLE_AUTOUPDATE", "true"},
		{"OPENCODE_DISABLE_DEFAULT_PLUGINS", "true"},
		{"OPENCODE_DISABLE_MODELS_FETCH", "true"},
		{"OPENCODE_DISABLE_CLAUDE_CODE", "true"},
		{"OPENCODE_AUTO_SHARE", "false"},
	} {
		value, valueErr := Set(pair.name, pair.value)
		if valueErr != nil {
			return Invocation{}, valueErr
		}
		values = append(values, value)
	}
	inline, err := Secret("OPENCODE_CONFIG_CONTENT", string(configuration))
	if err != nil {
		return Invocation{}, err
	}
	values = append(values, inline)
	environment, err := NewEnvironmentOverlay(values...)
	if err != nil {
		return Invocation{}, err
	}
	invocation := Invocation{Executable: options.Executable, ManagedArguments: managed, PassthroughArguments: passthrough, Environment: environment, IsolatedConfigDirectory: options.ConfigDirectory}
	return invocation, invocation.Validate()
}

func validateProviderOptions(options ProviderOptions) error {
	if options.Executable == "" {
		return fmt.Errorf("integration executable is required")
	}
	if strings.TrimSpace(options.Endpoint) == "" || (!strings.HasPrefix(options.Endpoint, "http://127.0.0.1:") && !strings.HasPrefix(options.Endpoint, "http://localhost:")) {
		return fmt.Errorf("Backpack integration endpoint must be loopback HTTP")
	}
	if strings.TrimSpace(options.Model) == "" {
		return fmt.Errorf("integration model is required")
	}
	if options.ConfigDirectory == "" || !filepath.IsAbs(options.ConfigDirectory) {
		return fmt.Errorf("isolated integration config directory must be absolute")
	}
	return nil
}

func rejectManagedArguments(integration string, arguments []string, managed ...string) error {
	keys := map[string]bool{}
	for _, key := range managed {
		keys[key] = true
	}
	for _, argument := range arguments {
		key := argument
		if before, _, found := strings.Cut(argument, "="); found {
			key = before
		}
		if keys[key] {
			return fmt.Errorf("%s argument %q conflicts with Backpack-managed provider routing", integration, argument)
		}
	}
	return nil
}

type CodexCatalogOptions struct {
	Model         catalog.Model
	ContextTokens int
	Path          string
}

func WriteCodexModelCatalog(options CodexCatalogOptions) error {
	if options.ContextTokens <= 0 {
		return fmt.Errorf("Codex model context must be known and greater than zero")
	}
	if options.Path == "" || !filepath.IsAbs(options.Path) {
		return fmt.Errorf("Codex model catalog path must be absolute")
	}
	modalities := []string{"text"}
	if hasCapability(options.Model.Capabilities, "vision") {
		modalities = append(modalities, "image")
	}
	payload := map[string]any{"models": []any{map[string]any{
		"slug": options.Model.ID, "display_name": options.Model.DisplayName, "context_window": options.ContextTokens,
		"shell_type": "default", "visibility": "list", "supported_in_api": true, "priority": 0,
		"truncation_policy": map[string]any{"mode": "bytes", "limit": 10000}, "input_modalities": modalities,
		"base_instructions": "", "support_verbosity": false, "supports_parallel_tool_calls": false,
		"supports_reasoning_summaries": false, "supported_reasoning_levels": []any{}, "experimental_supported_tools": []any{},
	}}}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(options.Path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(options.Path), ".models-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0600); err == nil {
		_, err = temporary.Write(data)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Remove(options.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, options.Path)
}
