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
)

const (
	claudeAppStateSchema = 1
	claudeAppProfileID   = "00000000-0000-4000-8000-000000000b4c"
)

type ClaudeAppOptions struct {
	StateDirectory string
	Endpoint       string
	APIKey         string
	Model          string
	ContextTokens  int
}

type claudeAppFileState struct {
	Path          string `json:"path"`
	Existed       bool   `json:"existed"`
	Original      []byte `json:"original,omitempty"`
	ManagedSHA256 string `json:"managed_sha256"`
}

type claudeAppState struct {
	SchemaVersion int                  `json:"schema_version"`
	Files         []claudeAppFileState `json:"files"`
}

func ConfigureClaudeApp(options ClaudeAppOptions) error {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return fmt.Errorf("Claude App launch is supported on Windows and macOS")
	}
	if options.StateDirectory == "" || !filepath.IsAbs(options.StateDirectory) || options.APIKey == "" || options.Model == "" || options.ContextTokens <= 0 {
		return fmt.Errorf("Claude App configuration is incomplete")
	}
	parsed, err := url.Parse(options.Endpoint)
	if err != nil || parsed.Scheme != "http" || (parsed.Hostname() != "127.0.0.1" && !strings.EqualFold(parsed.Hostname(), "localhost")) {
		return fmt.Errorf("Claude App endpoint must be loopback HTTP")
	}
	targets, err := claudeAppTargets()
	if err != nil {
		return err
	}
	statePath := filepath.Join(options.StateDirectory, "state.json")
	previous := map[string]claudeAppFileState{}
	if stateData, stateErr := os.ReadFile(statePath); stateErr == nil {
		var existing claudeAppState
		if json.Unmarshal(stateData, &existing) != nil || existing.SchemaVersion != claudeAppStateSchema {
			return fmt.Errorf("Claude App restore state is invalid")
		}
		for _, file := range existing.Files {
			current, existed, readErr := readRegularFile(file.Path)
			managed := readErr == nil && existed && claudeAppDigest(current) == file.ManagedSHA256
			if !managed && !(readErr == nil && existed && isClaudeAppModeConfig(file.Path) && claudeAppModeIntact(current)) {
				return fmt.Errorf("Claude App config changed after Backpack configured it; refusing to overwrite %s", file.Path)
			}
			previous[file.Path] = file
		}
	} else if !errors.Is(stateErr, os.ErrNotExist) {
		return stateErr
	}
	// Claude App appends /v1/models and /v1/messages to the configured gateway
	// root, so the managed base URL intentionally does not end in /v1.
	baseURL := strings.TrimRight(options.Endpoint, "/") + "/api/backpack/v1/integrations/claude-app/" + url.PathEscape(options.APIKey) + "/" + fmt.Sprint(options.ContextTokens) + "/" + url.PathEscape(options.Model)
	mutations := []struct {
		path string
		edit func(map[string]any)
	}{
		{targets.normalConfig, func(value map[string]any) { value["deploymentMode"] = "3p" }},
		{targets.thirdPartyConfig, func(value map[string]any) { value["deploymentMode"] = "3p" }},
		{targets.meta, func(value map[string]any) {
			value["appliedId"] = claudeAppProfileID
			value["entries"] = append(removeClaudeAppEntry(value["entries"], claudeAppProfileID), map[string]any{"id": claudeAppProfileID, "name": "Backpack"})
		}},
		{targets.profile, func(value map[string]any) {
			value["inferenceProvider"] = "gateway"
			value["inferenceGatewayBaseUrl"] = baseURL
			value["inferenceGatewayApiKey"] = "backpack-loopback"
			value["inferenceGatewayAuthScheme"] = "bearer"
			value["deploymentDisplayName"] = "Backpack"
			value["chatTabEnabled"] = true
			value["disableDeploymentModeChooser"] = true
			value["disableEssentialTelemetry"] = true
			value["disableNonessentialTelemetry"] = true
			value["autoModeEnabled"] = false
		}},
	}
	state := claudeAppState{SchemaVersion: claudeAppStateSchema}
	for _, mutation := range mutations {
		original, existed, readErr := readRegularFile(mutation.path)
		if readErr != nil {
			return readErr
		}
		value := map[string]any{}
		if existed && len(strings.TrimSpace(string(original))) > 0 {
			if err = json.Unmarshal(original, &value); err != nil {
				return fmt.Errorf("parse Claude App config %s: %w", mutation.path, err)
			}
		}
		mutation.edit(value)
		managed, _ := json.MarshalIndent(value, "", "  ")
		managed = append(managed, '\n')
		if err = writePrivateAtomic(mutation.path, managed); err != nil {
			return err
		}
		fileState := claudeAppFileState{Path: mutation.path, Existed: existed, Original: original, ManagedSHA256: claudeAppDigest(managed)}
		if saved, ok := previous[mutation.path]; ok {
			fileState.Existed = saved.Existed
			fileState.Original = saved.Original
		}
		state.Files = append(state.Files, fileState)
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err = writePrivateAtomic(statePath, append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func RestoreClaudeApp(stateDirectory string) error {
	statePath := filepath.Join(stateDirectory, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Claude App is not configured by Backpack")
		}
		return err
	}
	var state claudeAppState
	if err = json.Unmarshal(data, &state); err != nil || state.SchemaVersion != claudeAppStateSchema {
		return fmt.Errorf("Claude App restore state is invalid")
	}
	for _, file := range state.Files {
		current, existed, readErr := readRegularFile(file.Path)
		managed := readErr == nil && existed && claudeAppDigest(current) == file.ManagedSHA256
		if !managed && !(readErr == nil && existed && isClaudeAppModeConfig(file.Path) && claudeAppModeIntact(current)) {
			return fmt.Errorf("Claude App config changed after Backpack configured it; refusing destructive restore of %s", file.Path)
		}
	}
	for _, file := range state.Files {
		if isClaudeAppModeConfig(file.Path) {
			if err = restoreClaudeAppModeConfig(file); err != nil {
				return err
			}
			continue
		}
		if file.Existed {
			if err = writePrivateAtomic(file.Path, file.Original); err != nil {
				return err
			}
		} else if err = os.Remove(file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Remove(statePath)
}

func isClaudeAppModeConfig(path string) bool {
	return strings.EqualFold(filepath.Base(path), "claude_desktop_config.json")
}

func claudeAppModeIntact(data []byte) bool {
	var value map[string]any
	return json.Unmarshal(data, &value) == nil && value["deploymentMode"] == "3p"
}

// restoreClaudeAppModeConfig restores only the field Backpack owns. Claude may
// add preferences and Cowork paths while it runs; replacing the whole file
// would silently destroy those settings.
func restoreClaudeAppModeConfig(file claudeAppFileState) error {
	currentData, existed, err := readRegularFile(file.Path)
	if err != nil || !existed {
		if err != nil {
			return err
		}
		return fmt.Errorf("Claude App config disappeared after Backpack configured it: %s", file.Path)
	}
	current := map[string]any{}
	if err = json.Unmarshal(currentData, &current); err != nil {
		return fmt.Errorf("parse Claude App config %s: %w", file.Path, err)
	}
	original := map[string]any{}
	if file.Existed && len(strings.TrimSpace(string(file.Original))) > 0 {
		if err = json.Unmarshal(file.Original, &original); err != nil {
			return fmt.Errorf("parse saved Claude App config %s: %w", file.Path, err)
		}
	}
	if mode, ok := original["deploymentMode"]; ok {
		current["deploymentMode"] = mode
	} else {
		delete(current, "deploymentMode")
	}
	data, _ := json.MarshalIndent(current, "", "  ")
	return writePrivateAtomic(file.Path, append(data, '\n'))
}

type claudeAppPaths struct{ normalConfig, thirdPartyConfig, meta, profile string }

func claudeAppTargets() (claudeAppPaths, error) {
	var normalRoot, thirdRoot string
	if runtime.GOOS == "windows" {
		base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			return claudeAppPaths{}, fmt.Errorf("LOCALAPPDATA is unavailable")
		}
		normalRoot, thirdRoot = filepath.Join(base, "Claude"), filepath.Join(base, "Claude-3p")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return claudeAppPaths{}, err
		}
		normalRoot = filepath.Join(home, "Library", "Application Support", "Claude")
		thirdRoot = filepath.Join(home, "Library", "Application Support", "Claude-3p")
	}
	return claudeAppPaths{
		normalConfig: filepath.Join(normalRoot, "claude_desktop_config.json"), thirdPartyConfig: filepath.Join(thirdRoot, "claude_desktop_config.json"),
		meta: filepath.Join(thirdRoot, "configLibrary", "_meta.json"), profile: filepath.Join(thirdRoot, "configLibrary", claudeAppProfileID+".json"),
	}, nil
}

func removeClaudeAppEntry(raw any, id string) []any {
	items, _ := raw.([]any)
	result := make([]any, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if entryID, _ := entry["id"].(string); entryID == id {
			continue
		}
		result = append(result, item)
	}
	return result
}

func claudeAppDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func OpenClaudeApp() error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-a", "Claude").Start()
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("Claude App launch is supported on Windows and macOS")
	}
	base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	candidates := []string{filepath.Join(base, "Programs", "Claude", "Claude.exe"), filepath.Join(base, "Programs", "Claude Desktop", "Claude.exe"), filepath.Join(base, "Claude", "Claude.exe")}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return exec.Command(candidate).Start()
		}
	}
	// Current Windows Claude releases may be installed as MSIX packages under
	// WindowsApps, where direct executable discovery is intentionally restricted.
	// Claude registers this URI protocol for supported desktop installations.
	if err := exec.Command("explorer.exe", "claude://").Start(); err == nil {
		return nil
	}
	return fmt.Errorf("Claude App executable was not found; install and open Claude once, then retry")
}
