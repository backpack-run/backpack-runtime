package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/pelletier/go-toml/v2"
)

const codexAppStateSchema = 1

type CodexAppOptions struct {
	ConfigPath     string
	StateDirectory string
	Endpoint       string
	APIKey         string
	Model          catalog.Model
	ContextTokens  int
}

type codexAppState struct {
	SchemaVersion   int    `json:"schema_version"`
	ConfigPath      string `json:"config_path"`
	OriginalExisted bool   `json:"original_existed"`
	OriginalSHA256  string `json:"original_sha256,omitempty"`
	ManagedSHA256   string `json:"managed_sha256"`
	CatalogPath     string `json:"catalog_path"`
}

func DefaultCodexAppConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "config.toml"), nil
}

func ConfigureCodexApp(options CodexAppOptions) error {
	if err := validateCodexAppOptions(options); err != nil {
		return err
	}
	statePath := filepath.Join(options.StateDirectory, "state.json")
	backupPath := filepath.Join(options.StateDirectory, "config.original.toml")
	catalogPath := filepath.Join(options.StateDirectory, "models.json")

	current, existed, err := readRegularFile(options.ConfigPath)
	if err != nil {
		return err
	}
	if state, stateErr := readCodexAppState(statePath); stateErr == nil {
		if !samePath(state.ConfigPath, options.ConfigPath) {
			return fmt.Errorf("Codex App restore state belongs to a different config path: %s", state.ConfigPath)
		}
		if digest(current) != state.ManagedSHA256 {
			return fmt.Errorf("Codex App config changed after Backpack configured it; refusing to overwrite %s (restore state: %s)", options.ConfigPath, statePath)
		}
		original, _, backupErr := readRegularFile(backupPath)
		if backupErr != nil || digest(original) != state.OriginalSHA256 {
			return fmt.Errorf("Codex App backup does not match restore state; refusing to reconfigure")
		}
		existed = state.OriginalExisted
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return stateErr
	}

	managed, err := patchCodexAppConfig(current, options.Model.ID, catalogPath, codexAppBaseURL(options.Endpoint, options.APIKey))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(options.StateDirectory, 0700); err != nil {
		return err
	}
	newState := codexAppState{SchemaVersion: codexAppStateSchema, ConfigPath: options.ConfigPath, OriginalExisted: existed, ManagedSHA256: digest(managed), CatalogPath: catalogPath}
	if _, stateErr := readCodexAppState(statePath); errors.Is(stateErr, os.ErrNotExist) {
		newState.OriginalSHA256 = digest(current)
		if err = writePrivateAtomic(backupPath, current); err != nil {
			return fmt.Errorf("back up Codex App config: %w", err)
		}
	} else {
		previous, stateErr := readCodexAppState(statePath)
		if stateErr != nil {
			return stateErr
		}
		newState.OriginalSHA256 = previous.OriginalSHA256
		newState.OriginalExisted = previous.OriginalExisted
	}
	if err = WriteCodexModelCatalog(CodexCatalogOptions{Model: options.Model, ContextTokens: options.ContextTokens, Path: catalogPath}); err != nil {
		return err
	}
	if err = writePrivateAtomic(options.ConfigPath, managed); err != nil {
		return fmt.Errorf("write Codex App config: %w", err)
	}
	stateData, _ := json.MarshalIndent(newState, "", "  ")
	if err = writePrivateAtomic(statePath, append(stateData, '\n')); err != nil {
		_ = restoreCodexAppConfig(options.ConfigPath, backupPath, newState.OriginalExisted)
		return fmt.Errorf("write Codex App restore state: %w", err)
	}
	return nil
}

func RestoreCodexApp(configPath, stateDirectory string) error {
	statePath := filepath.Join(stateDirectory, "state.json")
	state, err := readCodexAppState(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Codex App is not configured by Backpack")
		}
		return err
	}
	if !samePath(configPath, state.ConfigPath) {
		return fmt.Errorf("Codex App restore state belongs to a different config path: %s", state.ConfigPath)
	}
	current, _, err := readRegularFile(configPath)
	if err != nil {
		return err
	}
	if digest(current) != state.ManagedSHA256 {
		return fmt.Errorf("Codex App config changed after Backpack configured it; refusing destructive restore of %s (restore state: %s)", configPath, statePath)
	}
	backupPath := filepath.Join(stateDirectory, "config.original.toml")
	original, _, err := readRegularFile(backupPath)
	if err != nil {
		return fmt.Errorf("read Codex App backup: %w", err)
	}
	if digest(original) != state.OriginalSHA256 {
		return fmt.Errorf("Codex App backup checksum does not match restore state")
	}
	if err = restoreCodexAppConfig(configPath, backupPath, state.OriginalExisted); err != nil {
		return err
	}
	if err = os.Remove(state.CatalogPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func OpenCodexApp() error {
	const launchURL = "codex://threads/new?mode=codex"
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", launchURL)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("open Codex App: %w", err)
		}
		return cmd.Process.Release()
	case "darwin":
		if err := exec.Command("open", launchURL).Run(); err != nil {
			return fmt.Errorf("open Codex App: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("Codex App launch is supported on Windows and macOS")
	}
}

func codexAppBaseURL(endpoint, apiKey string) string {
	return strings.TrimRight(endpoint, "/") + "/api/backpack/v1/integrations/codex-app/" + url.PathEscape(apiKey) + "/v1"
}

func validateCodexAppOptions(options CodexAppOptions) error {
	if options.ConfigPath == "" || !filepath.IsAbs(options.ConfigPath) {
		return fmt.Errorf("Codex App config path must be absolute")
	}
	if options.StateDirectory == "" || !filepath.IsAbs(options.StateDirectory) {
		return fmt.Errorf("Codex App state directory must be absolute")
	}
	if strings.TrimSpace(options.Model.ID) == "" {
		return fmt.Errorf("Codex App model is required")
	}
	if options.ContextTokens <= 0 {
		return fmt.Errorf("Codex App model context must be known")
	}
	if strings.TrimSpace(options.APIKey) == "" {
		return fmt.Errorf("Backpack daemon API key is required")
	}
	if strings.TrimSpace(options.Endpoint) == "" || (!strings.HasPrefix(options.Endpoint, "http://127.0.0.1:") && !strings.HasPrefix(options.Endpoint, "http://localhost:")) {
		return fmt.Errorf("Codex App endpoint must be loopback HTTP")
	}
	return nil
}

func patchCodexAppConfig(input []byte, model, catalogPath, baseURL string) ([]byte, error) {
	var parsed map[string]any
	if len(strings.TrimSpace(string(input))) > 0 {
		if err := toml.Unmarshal(input, &parsed); err != nil {
			return nil, fmt.Errorf("invalid Codex config TOML: %w", err)
		}
	}
	text := strings.ReplaceAll(string(input), "\r\n", "\n")
	for _, key := range []string{"model", "model_provider", "model_catalog_json", "openai_base_url", "profile"} {
		var err error
		text, err = removeRootStringAssignment(text, key)
		if err != nil {
			return nil, err
		}
	}
	assignments := fmt.Sprintf("model = %q\nmodel_catalog_json = %q\nopenai_base_url = %q\n", model, catalogPath, baseURL)
	insert := strings.Index(text, "\n[")
	if insert < 0 && strings.HasPrefix(strings.TrimSpace(text), "[") {
		insert = 0
	}
	if insert < 0 {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += assignments
	} else {
		if insert > 0 {
			insert++
		}
		text = text[:insert] + assignments + text[insert:]
	}
	parsed = map[string]any{}
	if err := toml.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("generated invalid Codex App config: %w", err)
	}
	for key, want := range map[string]string{"model": model, "model_catalog_json": catalogPath, "openai_base_url": baseURL} {
		if got, ok := parsed[key].(string); !ok || got != want {
			return nil, fmt.Errorf("generated Codex App config omitted %s", key)
		}
	}
	if _, exists := parsed["model_provider"]; exists {
		return nil, fmt.Errorf("generated Codex App config retained model_provider")
	}
	return []byte(text), nil
}

func removeRootStringAssignment(text, key string) (string, error) {
	lines := strings.SplitAfter(text, "\n")
	offset := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "#") {
			break
		}
		if rootAssignmentKey(trimmed) == key {
			if strings.Contains(line, `'''`) || strings.Contains(line, `"""`) {
				return "", fmt.Errorf("Codex config uses an unsupported multiline value for managed key %s", key)
			}
			return text[:offset] + text[offset+len(line):], nil
		}
		offset += len(line)
	}
	return text, nil
}

func rootAssignmentKey(line string) string {
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	key, _, ok := strings.Cut(line, "=")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(key), `"'`)
}

func readCodexAppState(path string) (codexAppState, error) {
	var state codexAppState
	data, exists, err := readRegularFile(path)
	if err != nil {
		return state, err
	}
	if !exists {
		return state, os.ErrNotExist
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode Codex App restore state: %w", err)
	}
	if state.SchemaVersion != codexAppStateSchema || state.ConfigPath == "" || state.ManagedSHA256 == "" || state.CatalogPath == "" {
		return state, fmt.Errorf("invalid Codex App restore state")
	}
	return state, nil
}

func readRegularFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("refusing non-regular file %s", path)
	}
	data, err := os.ReadFile(path)
	return data, true, err
}

func writePrivateAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("refusing to replace non-regular file %s", path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".backpack-codex-app-*")
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
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func restoreCodexAppConfig(configPath, backupPath string, existed bool) error {
	if !existed {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	original, _, err := readRegularFile(backupPath)
	if err != nil {
		return err
	}
	return writePrivateAtomic(configPath, original)
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
