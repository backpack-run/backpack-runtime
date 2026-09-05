package pythonruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"github.com/backpack-run/backpack-runtime/internal/runtimebundle"
)

//go:embed workers/*.py requirements/*.txt requirements/*.lock
var assets embed.FS

const protocolVersion = 1
const environmentSchemaVersion = 3
const dependencyLockVersion = 1

type Definition struct {
	Engine, Version, Python, Worker, Lock, Platform string
	Index, ExtraIndex                               string
}

type Environment struct {
	Engine, Version, Python, Worker, Directory, UV string
}

type Manager struct {
	Paths    config.Paths
	Runtimes *runtimebundle.Manager
	mu       sync.Mutex
}

var definitions = map[string]Definition{
	"qwen-asr": {Engine: "qwen-asr", Version: "0.0.6", Python: "3.11", Worker: "qwen_asr_worker.py", Lock: "qwen-asr-0.0.6-windows-amd64.lock", Platform: "windows-amd64-cpu", Index: "https://pypi.org/simple", ExtraIndex: "https://download.pytorch.org/whl/cpu"},
	"kokoro":   {Engine: "kokoro", Version: "0.9.4", Python: "3.12", Worker: "kokoro_worker.py", Lock: "kokoro-0.9.4-windows-amd64.lock", Platform: "windows-amd64-cpu", Index: "https://pypi.org/simple", ExtraIndex: "https://download.pytorch.org/whl/cpu"},
}

func (m *Manager) Ensure(ctx context.Context, requirement models.RuntimeRequirement, target compute.Target) (*Environment, error) {
	definition, ok := definitions[strings.ToLower(requirement.Engine)]
	if !ok || requirement.Version != definition.Version {
		return nil, fmt.Errorf("isolated Python runtime %s %s is not in the trusted environment catalog", requirement.Engine, requirement.Version)
	}
	if target.Kind() != "local" {
		return nil, fmt.Errorf("isolated Python runtime %s %s is not yet available on %s compute", requirement.Engine, requirement.Version, target.Kind())
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return nil, fmt.Errorf("isolated Python runtime %s %s is not available for %s/%s", requirement.Engine, requirement.Version, runtime.GOOS, runtime.GOARCH)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	root := filepath.Join(m.Paths.Runtimes, "python", "environments", definition.Engine, definition.Version, definition.Platform)
	python := filepath.Join(root, "environment", "Scripts", "python.exe")
	worker := filepath.Join(root, definition.Worker)
	uv, err := m.Runtimes.Ensure(ctx, models.RuntimeRequirement{Engine: "uv", Version: "0.11.15", Environment: "native-bundle"}, target)
	if err != nil {
		return nil, fmt.Errorf("install managed Python bootstrap: %w", err)
	}
	if validEnvironment(root, python, worker, definition) {
		if err = repairPythonRedirector(python); err != nil {
			return nil, err
		}
		return &Environment{definition.Engine, definition.Version, python, worker, root, uv.Executable}, nil
	}
	events.Emit(m.Runtimes.Sink, events.Event{Type: events.RuntimePreparing, Kind: events.Status, Message: "Creating isolated " + definition.Engine + " environment"})
	staging := fmt.Sprintf("%s.partial-%d", root, os.Getpid())
	if err = os.RemoveAll(staging); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(staging, 0700); err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	requirements, _ := assets.ReadFile("requirements/" + definition.Lock)
	workerData, _ := assets.ReadFile("workers/" + definition.Worker)
	requirementsPath := filepath.Join(staging, "requirements.lock")
	workerPath := filepath.Join(staging, definition.Worker)
	if err = os.WriteFile(requirementsPath, requirements, 0600); err != nil {
		return nil, err
	}
	if err = os.WriteFile(workerPath, workerData, 0600); err != nil {
		return nil, err
	}
	environmentPath := filepath.Join(staging, "environment")
	cache := filepath.Join(m.Paths.Cache, "uv")
	distributions := filepath.Join(m.Paths.Runtimes, "python", "distributions")
	if err = os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	uvEnvironment := []string{"UV_CACHE_DIR=" + cache, "UV_PYTHON_INSTALL_DIR=" + distributions, "UV_NO_CONFIG=1"}
	if err = execute(ctx, target, compute.Command{Executable: uv.Executable, Args: []string{"venv", "--python", definition.Python, "--python-preference", "only-managed", environmentPath}, Env: uvEnvironment}); err != nil {
		return nil, fmt.Errorf("create managed Python %s environment: %w", definition.Engine, err)
	}
	stagingPython := filepath.Join(environmentPath, "Scripts", "python.exe")
	// PyTorch's CPU wheel repository mirrors some common packages. Exact pins
	// plus all-or-nothing hashes make multi-index selection deterministic while
	// preventing an untrusted same-name distribution from being installed.
	args := []string{"pip", "install", "--python", stagingPython, "--require-hashes", "--index-strategy", "unsafe-best-match", "--index-url", definition.Index}
	if definition.ExtraIndex != "" {
		args = append(args, "--extra-index-url", definition.ExtraIndex)
	}
	args = append(args, "-r", requirementsPath)
	if err = execute(ctx, target, compute.Command{Executable: uv.Executable, Args: args, Env: uvEnvironment}); err != nil {
		return nil, fmt.Errorf("install pinned %s dependencies: %w", definition.Engine, err)
	}
	manifest := environmentManifest(definition, requirements, workerData)
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err = os.WriteFile(filepath.Join(staging, "environment.json"), data, 0600); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return nil, err
	}
	_ = os.RemoveAll(root)
	if err = os.Rename(staging, root); err != nil {
		return nil, fmt.Errorf("publish Python environment: %w", err)
	}
	events.Emit(m.Runtimes.Sink, events.Event{Type: events.RuntimeInstalled, Kind: events.Complete, Message: "Installed isolated " + definition.Engine + " environment"})
	return &Environment{definition.Engine, definition.Version, python, worker, root, uv.Executable}, nil
}

func execute(ctx context.Context, target compute.Target, command compute.Command) error {
	var stdout, stderr bytes.Buffer
	if command.Stdout == nil {
		command.Stdout = &stdout
	}
	if command.Stderr == nil {
		command.Stderr = &stderr
	}
	process, err := target.Execute(ctx, command)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	select {
	case err = <-done:
		if err != nil {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = strings.TrimSpace(stdout.String())
			}
			return fmt.Errorf("%w: %s", err, message)
		}
		return nil
	case <-ctx.Done():
		_ = process.Stop(context.Background())
		return ctx.Err()
	}
}

type manifest struct {
	SchemaVersion   int                  `json:"schema_version"`
	LockVersion     int                  `json:"dependency_lock_version"`
	ProtocolVersion int                  `json:"protocol_version"`
	Engine          string               `json:"engine"`
	Version         string               `json:"version"`
	Python          string               `json:"python"`
	Platform        string               `json:"platform"`
	Indexes         []string             `json:"indexes"`
	Assets          map[string]string    `json:"assets"`
	Distributions   []lockedDistribution `json:"resolved_distributions"`
}

type lockedDistribution struct {
	Requirement string   `json:"requirement"`
	Hashes      []string `json:"sha256"`
}

func environmentManifest(d Definition, requirements, worker []byte) manifest {
	indexes := []string{d.Index}
	if d.ExtraIndex != "" {
		indexes = append(indexes, d.ExtraIndex)
	}
	return manifest{SchemaVersion: environmentSchemaVersion, LockVersion: dependencyLockVersion, ProtocolVersion: protocolVersion, Engine: d.Engine, Version: d.Version, Python: d.Python, Platform: d.Platform, Indexes: indexes, Assets: map[string]string{"requirements.lock": digest(requirements), d.Worker: digest(worker)}, Distributions: parseLockedDistributions(requirements)}
}
func validEnvironment(root, python, worker string, d Definition) bool {
	data, err := os.ReadFile(filepath.Join(root, "environment.json"))
	if err != nil {
		return false
	}
	var got manifest
	if json.Unmarshal(data, &got) != nil {
		return false
	}
	requirements, _ := assets.ReadFile("requirements/" + d.Lock)
	workerData, _ := assets.ReadFile("workers/" + d.Worker)
	want := environmentManifest(d, requirements, workerData)
	if got.SchemaVersion != environmentSchemaVersion || got.LockVersion != dependencyLockVersion || got.Engine != want.Engine || got.Version != want.Version || got.Python != want.Python || got.Platform != want.Platform || got.ProtocolVersion != protocolVersion || !slices.Equal(got.Indexes, want.Indexes) || got.Assets["requirements.lock"] != want.Assets["requirements.lock"] || got.Assets[d.Worker] != want.Assets[d.Worker] || len(got.Distributions) != len(want.Distributions) {
		return false
	}
	_, pythonErr := os.Stat(python)
	if os.IsNotExist(pythonErr) {
		_, pythonErr = os.Stat(filepath.Join(filepath.Dir(python), "pythonw.exe"))
	}
	_, workerErr := os.Stat(worker)
	return pythonErr == nil && workerErr == nil
}

func parseLockedDistributions(data []byte) []lockedDistribution {
	var out []lockedDistribution
	var current *lockedDistribution
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "--hash=") && (strings.Contains(line, "==") || strings.Contains(line, " @ ")) {
			requirement := strings.TrimSpace(strings.TrimSuffix(line, "\\"))
			out = append(out, lockedDistribution{Requirement: requirement})
			current = &out[len(out)-1]
			continue
		}
		if current != nil && strings.HasPrefix(line, "--hash=sha256:") {
			hash := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "--hash=sha256:"), "\\"))
			current.Hashes = append(current.Hashes, hash)
		}
	}
	return out
}
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func repairPythonRedirector(python string) error {
	if _, err := os.Stat(python); err == nil {
		return nil
	}
	source, err := os.Open(filepath.Join(filepath.Dir(python), "pythonw.exe"))
	if err != nil {
		return fmt.Errorf("managed Python executable is missing: %w", err)
	}
	defer source.Close()
	destination, err := os.OpenFile(python, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	if _, err = io.Copy(destination, source); err != nil {
		_ = destination.Close()
		_ = os.Remove(python)
		return err
	}
	return destination.Close()
}
