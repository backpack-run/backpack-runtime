package compute

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/backpack-run/backpack-runtime/internal/models"
)

type SSHConfig struct {
	ID           string `json:"name"`
	Host         string `json:"host"`
	User         string `json:"user,omitempty"`
	Port         int    `json:"port,omitempty"`
	IdentityFile string `json:"identity_file,omitempty"`
	RemoteRoot   string `json:"remote_root,omitempty"`
}
type SSHRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	Start(string, []string, io.Writer, io.Writer) (Process, error)
}
type execSSHRunner struct{}

func (execSSHRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
func (execSSHRunner) Start(name string, args []string, stdout, stderr io.Writer) (Process, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	cleanup, err := attachProcessLifetime(cmd.Process)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	p := &localProcess{cmd: cmd, done: make(chan struct{}), cleanup: cleanup}
	go func() { p.err = cmd.Wait(); p.cleanup(); close(p.done) }()
	return p, nil
}

type SSHTarget struct {
	Config   SSHConfig
	Runner   SSHRunner
	mu       sync.Mutex
	home     string
	mappings map[string]string
}

func NewSSH(config SSHConfig) *SSHTarget {
	if config.Port == 0 {
		config.Port = 22
	}
	if config.RemoteRoot == "" {
		config.RemoteRoot = ".backpack"
	}
	return &SSHTarget{Config: config, Runner: execSSHRunner{}, mappings: map[string]string{}}
}
func (s *SSHTarget) Name() string { return s.Config.ID }
func (s *SSHTarget) Kind() string { return "ssh" }
func (s *SSHTarget) Prepare(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	out, err := s.run(ctx, `printf 'BP_HOME=%s\n' "$HOME"; command -v llama-server >/dev/null || { echo BP_MISSING_LLAMA=1; exit 42; }; command -v sha256sum >/dev/null`)
	if err != nil {
		if strings.Contains(string(out), "BP_MISSING_LLAMA=1") {
			return fmt.Errorf("remote target %q has no llama-server on PATH; install the manifest-compatible llama.cpp runtime", s.Name())
		}
		return fmt.Errorf("SSH readiness for %q failed: %w: %s", s.Name(), err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "BP_HOME=") {
			s.mu.Lock()
			s.home = strings.TrimPrefix(line, "BP_HOME=")
			s.mu.Unlock()
		}
	}
	return nil
}
func (s *SSHTarget) ResolvePath(local string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	bestLocal, bestRemote := "", ""
	for l, r := range s.mappings {
		if strings.HasPrefix(strings.ToLower(local), strings.ToLower(l)) && len(l) > len(bestLocal) {
			bestLocal, bestRemote = l, r
		}
	}
	if bestLocal == "" {
		return local
	}
	rel, _ := filepath.Rel(bestLocal, local)
	return bestRemote + "/" + filepath.ToSlash(rel)
}
func (s *SSHTarget) PrepareModel(ctx context.Context, m *models.Installed) error {
	s.mu.Lock()
	home := s.home
	s.mu.Unlock()
	if home == "" {
		if err := s.Prepare(ctx); err != nil {
			return err
		}
		s.mu.Lock()
		home = s.home
		s.mu.Unlock()
	}
	remote := strings.TrimRight(home, "/") + "/" + strings.Trim(s.Config.RemoteRoot, "/") + "/models/" + m.ID + "/" + m.Revision + "/" + m.Package.ID
	if _, err := s.run(ctx, "mkdir -p "+shellQuote(remote)); err != nil {
		return fmt.Errorf("create remote model cache: %w", err)
	}
	for _, file := range m.Package.ArtifactFiles() {
		local := filepath.Join(m.Directory, filepath.FromSlash(file.Filename))
		remoteFile := remote + "/" + filepath.ToSlash(file.Filename)
		check := "test -f " + shellQuote(remoteFile) + " && test \"$(sha256sum " + shellQuote(remoteFile) + " | awk '{print $1}')\" = " + shellQuote(strings.ToLower(file.SHA256))
		if _, err := s.run(ctx, check); err == nil {
			continue
		}
		if _, err := s.run(ctx, "mkdir -p "+shellQuote(remoteFile[:strings.LastIndex(remoteFile, "/")])); err != nil {
			return err
		}
		part := remoteFile + ".part"
		if out, err := s.copy(ctx, local, part); err != nil {
			return fmt.Errorf("copy %s to %s: %w: %s", file.Filename, s.Name(), err, strings.TrimSpace(string(out)))
		}
		verify := "test \"$(sha256sum " + shellQuote(part) + " | awk '{print $1}')\" = " + shellQuote(strings.ToLower(file.SHA256)) + " && mv -f " + shellQuote(part) + " " + shellQuote(remoteFile)
		if out, err := s.run(ctx, verify); err != nil {
			return fmt.Errorf("remote checksum verification failed for %s: %w: %s", file.Filename, err, strings.TrimSpace(string(out)))
		}
	}
	s.mu.Lock()
	s.mappings[m.Directory] = remote
	s.mu.Unlock()
	return nil
}
func (s *SSHTarget) Inspect(ctx context.Context) (Hardware, error) {
	script := `printf 'BP_OS='; (. /etc/os-release 2>/dev/null; printf '%s\n' "${PRETTY_NAME:-Linux}"); printf 'BP_ARCH='; uname -m; printf 'BP_CPU='; (lscpu 2>/dev/null | sed -n 's/^Model name:[[:space:]]*//p' | head -1); printf 'BP_CORES='; getconf _NPROCESSORS_ONLN; printf 'BP_RAM_KB='; awk '/MemTotal/{print $2}' /proc/meminfo; printf 'BP_DISK_KB='; df -Pk "$HOME" | awk 'NR==2{print $4}'; printf 'BP_RUNTIME='; (command -v llama-server >/dev/null && printf yes || printf no); printf '\n'; if command -v nvidia-smi >/dev/null; then nvidia-smi --query-gpu=name,memory.total --format=csv,noheader,nounits | sed 's/^/BP_GPU=/' ; fi; printf 'BP_CUDA='; (command -v nvcc >/dev/null && printf yes || printf no); printf '\n'`
	out, err := s.run(ctx, script)
	if err != nil {
		return Hardware{}, fmt.Errorf("inspect SSH target: %w: %s", err, strings.TrimSpace(string(out)))
	}
	h := Hardware{Backends: []string{"cpu"}}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "BP_OS":
			h.OS = value
		case "BP_ARCH":
			h.Architecture = value
		case "BP_CPU":
			h.CPU = value
		case "BP_CORES":
			h.Cores, _ = strconv.Atoi(value)
		case "BP_RAM_KB":
			kb, _ := strconv.ParseFloat(value, 64)
			h.MemoryTotalGB = kb / 1048576
		case "BP_DISK_KB":
			kb, _ := strconv.ParseFloat(value, 64)
			h.DiskAvailableGB = kb / 1048576
		case "BP_RUNTIME":
			h.RuntimeReady = value == "yes"
		case "BP_CUDA":
			if value == "yes" {
				h.Backends = append(h.Backends, "cuda")
			}
		case "BP_GPU":
			parts := strings.Split(value, ",")
			gpu := GPU{Vendor: "nvidia", Model: strings.TrimSpace(parts[0])}
			if len(parts) > 1 {
				mb, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				gpu.VRAMGB = mb / 1024
			}
			h.GPUs = append(h.GPUs, gpu)
		}
	}
	return h, nil
}
func (s *SSHTarget) Execute(_ context.Context, c Command) (Process, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	port := ""
	for i, arg := range c.Args {
		if arg == "--port" && i+1 < len(c.Args) {
			port = c.Args[i+1]
		}
	}
	if port == "" {
		return nil, fmt.Errorf("remote command has no loopback port")
	}
	remote := shellQuote(c.Executable)
	for _, arg := range c.Args {
		remote += " " + shellQuote(arg)
	}
	args := s.baseArgs()
	args = append(args, "-L", "127.0.0.1:"+port+":127.0.0.1:"+port, s.destination(), "--", remote)
	return s.Runner.Start("ssh", args, c.Stdout, c.Stderr)
}
func (s *SSHTarget) validate() error {
	if s.Config.ID == "" || s.Config.Host == "" {
		return fmt.Errorf("SSH target name and host are required")
	}
	if !validSSHPart(s.Config.ID) || !validSSHPart(s.Config.Host) || (s.Config.User != "" && !validSSHPart(s.Config.User)) || s.Config.Port < 1 || s.Config.Port > 65535 || !validRemoteRoot(s.Config.RemoteRoot) {
		return fmt.Errorf("invalid SSH target configuration")
	}
	if s.Config.IdentityFile != "" {
		if _, err := os.Stat(s.Config.IdentityFile); err != nil {
			return fmt.Errorf("SSH identity file: %w", err)
		}
	}
	return nil
}
func validSSHPart(value string) bool {
	return value != "" && len(value) <= 255 && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".-_:", r))
	}) < 0
}
func validRemoteRoot(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") {
		return false
	}
	for _, part := range strings.Split(strings.ReplaceAll(value, "\\", "/"), "/") {
		if part == "" || part == ".." || !validSSHPart(part) {
			return false
		}
	}
	return true
}
func (s *SSHTarget) destination() string {
	if s.Config.User != "" {
		return s.Config.User + "@" + s.Config.Host
	}
	return s.Config.Host
}
func (s *SSHTarget) baseArgs() []string {
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=12", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3", "-o", "StrictHostKeyChecking=yes"}
	if s.Config.IdentityFile != "" {
		args = append(args, "-i", s.Config.IdentityFile)
	}
	if s.Config.Port != 22 {
		args = append(args, "-p", strconv.Itoa(s.Config.Port))
	}
	return args
}
func (s *SSHTarget) run(ctx context.Context, script string) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	args := append(s.baseArgs(), s.destination(), "--", script)
	return s.Runner.Run(ctx, "ssh", args...)
}
func (s *SSHTarget) copy(ctx context.Context, local, remote string) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes"}
	if s.Config.IdentityFile != "" {
		args = append(args, "-i", s.Config.IdentityFile)
	}
	if s.Config.Port != 22 {
		args = append(args, "-P", strconv.Itoa(s.Config.Port))
	}
	args = append(args, local, s.destination()+":"+remote)
	return s.Runner.Run(ctx, "scp", args...)
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
