package compute

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/models"
)

type Hardware struct {
	OS              string   `json:"os"`
	Architecture    string   `json:"architecture"`
	CPU             string   `json:"cpu"`
	Cores           int      `json:"cores"`
	MemoryTotalGB   float64  `json:"memory_total_gb,omitempty"`
	DiskAvailableGB float64  `json:"disk_available_gb,omitempty"`
	RuntimeReady    bool     `json:"runtime_ready"`
	GPUs            []GPU    `json:"gpus"`
	Backends        []string `json:"backends"`
}
type GPU struct {
	Vendor string  `json:"vendor"`
	Model  string  `json:"model"`
	VRAMGB float64 `json:"vram_gb,omitempty"`
}
type Command struct {
	Executable     string
	Args           []string
	Env            []string
	Dir            string
	Stdout, Stderr io.Writer
}
type Process interface {
	PID() int
	Wait() error
	Stop(context.Context) error
}
type Target interface {
	Name() string
	Kind() string
	Inspect(context.Context) (Hardware, error)
	Prepare(context.Context) error
	PrepareModel(context.Context, *models.Installed) error
	ResolvePath(string) string
	Execute(context.Context, Command) (Process, error)
}

type Local struct{}

func (Local) Name() string                                          { return "local" }
func (Local) Kind() string                                          { return "local" }
func (Local) Prepare(context.Context) error                         { return nil }
func (Local) PrepareModel(context.Context, *models.Installed) error { return nil }
func (Local) ResolvePath(path string) string                        { return path }
func (Local) Inspect(ctx context.Context) (Hardware, error) {
	h := Hardware{OS: runtime.GOOS, Architecture: runtime.GOARCH, Cores: runtime.NumCPU(), Backends: []string{"cpu"}}
	if runtime.GOOS == "windows" {
		out, _ := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "$c=Get-CimInstance Win32_ComputerSystem;$p=Get-CimInstance Win32_Processor|Select-Object -First 1;$d=Get-PSDrive -Name ([IO.Path]::GetPathRoot($env:LOCALAPPDATA).TrimEnd('\\').TrimEnd(':'));\"$($p.Name)|$($c.TotalPhysicalMemory)|$($d.Free)\"").Output()
		parts := strings.Split(strings.TrimSpace(string(out)), "|")
		if len(parts) >= 2 {
			h.CPU = parts[0]
			fmt.Sscanf(parts[1], "%f", &h.MemoryTotalGB)
			h.MemoryTotalGB /= 1073741824
		}
		if len(parts) >= 3 {
			fmt.Sscanf(parts[2], "%f", &h.DiskAvailableGB)
			h.DiskAvailableGB /= 1073741824
		}
		gpu, _ := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Get-CimInstance Win32_VideoController|Select-Object -ExpandProperty Name").Output()
		for _, name := range strings.Split(strings.TrimSpace(string(gpu)), "\n") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			vendor := "other"
			lower := strings.ToLower(name)
			if strings.Contains(lower, "nvidia") {
				vendor = "nvidia"
				h.Backends = append(h.Backends, "cuda", "vulkan")
			} else if strings.Contains(lower, "amd") || strings.Contains(lower, "radeon") {
				vendor = "amd"
				h.Backends = append(h.Backends, "vulkan", "rocm")
			} else if strings.Contains(lower, "intel") {
				vendor = "intel"
				h.Backends = append(h.Backends, "vulkan")
			}
			h.GPUs = append(h.GPUs, GPU{Vendor: vendor, Model: name})
		}
	}
	if os.Getenv("BACKPACK_LLAMA_SERVER") != "" {
		h.RuntimeReady = true
	} else if _, err := exec.LookPath("llama-server"); err == nil {
		h.RuntimeReady = true
	}
	if h.CPU == "" {
		h.CPU = "unknown"
	}
	fillPlatformHardware(ctx, &h)
	return h, nil
}
func (Local) Execute(ctx context.Context, s Command) (Process, error) {
	// Process lifetime belongs to the session manager, not to the HTTP request
	// that created the session. The context is used by Prepare/Stop boundaries.
	_ = ctx
	cmd := exec.Command(s.Executable, s.Args...)
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Dir = s.Dir
	cmd.Stdin = nil
	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	cleanup, err := attachProcessLifetime(cmd.Process)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("attach child lifetime: %w", err)
	}
	p := &localProcess{cmd: cmd, done: make(chan struct{}), cleanup: cleanup}
	go func() { p.err = cmd.Wait(); p.cleanup(); close(p.done) }()
	return p, nil
}

type localProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	err     error
	cleanup func()
}

func (p *localProcess) PID() int    { return p.cmd.Process.Pid }
func (p *localProcess) Wait() error { <-p.done; return p.err }
func (p *localProcess) Stop(ctx context.Context) error {
	if p.cmd.Process == nil {
		return nil
	}
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		if killErr := p.cmd.Process.Kill(); killErr != nil {
			return killErr
		}
		<-p.done
		return nil
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		if err := p.cmd.Process.Kill(); err != nil {
			return err
		}
		<-p.done
		return nil
	}
}

type Managed struct{}

func (Managed) Name() string { return "backpack" }
func (Managed) Kind() string { return "managed" }
func (Managed) Prepare(context.Context) error {
	return fmt.Errorf("Backpack managed compute has no control plane yet")
}
func (Managed) PrepareModel(context.Context, *models.Installed) error {
	return fmt.Errorf("Backpack managed compute has no control plane yet")
}
func (Managed) ResolvePath(path string) string { return path }
func (Managed) Inspect(context.Context) (Hardware, error) {
	return Hardware{}, fmt.Errorf("Backpack managed compute has no control plane yet")
}
func (Managed) Execute(context.Context, Command) (Process, error) {
	return nil, fmt.Errorf("Backpack managed compute has no control plane yet")
}
