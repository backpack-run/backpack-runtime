package compute

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Hardware struct {
	OS            string   `json:"os"`
	Architecture  string   `json:"architecture"`
	CPU           string   `json:"cpu"`
	Cores         int      `json:"cores"`
	MemoryTotalGB float64  `json:"memory_total_gb,omitempty"`
	GPUs          []GPU    `json:"gpus"`
	Backends      []string `json:"backends"`
}
type GPU struct {
	Vendor, Model string
	VRAMGB        float64 `json:"vram_gb,omitempty"`
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
	Execute(context.Context, Command) (Process, error)
}

type Local struct{}

func (Local) Name() string                  { return "local" }
func (Local) Kind() string                  { return "local" }
func (Local) Prepare(context.Context) error { return nil }
func (Local) Inspect(ctx context.Context) (Hardware, error) {
	h := Hardware{OS: runtime.GOOS, Architecture: runtime.GOARCH, Cores: runtime.NumCPU(), Backends: []string{"cpu"}}
	if runtime.GOOS == "windows" {
		out, _ := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "$c=Get-CimInstance Win32_ComputerSystem;$p=Get-CimInstance Win32_Processor|Select-Object -First 1;\"$($p.Name)|$($c.TotalPhysicalMemory)\"").Output()
		parts := strings.Split(strings.TrimSpace(string(out)), "|")
		if len(parts) == 2 {
			h.CPU = parts[0]
			fmt.Sscanf(parts[1], "%f", &h.MemoryTotalGB)
			h.MemoryTotalGB /= 1073741824
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
	if h.CPU == "" {
		h.CPU = "unknown"
	}
	return h, nil
}
func (Local) Execute(ctx context.Context, s Command) (Process, error) {
	cmd := exec.CommandContext(ctx, s.Executable, s.Args...)
	cmd.Env = append(os.Environ(), s.Env...)
	cmd.Dir = s.Dir
	cmd.Stdin = nil
	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &localProcess{cmd}, nil
}

type localProcess struct{ cmd *exec.Cmd }

func (p *localProcess) PID() int    { return p.cmd.Process.Pid }
func (p *localProcess) Wait() error { return p.cmd.Wait() }
func (p *localProcess) Stop(ctx context.Context) error {
	if p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return p.cmd.Process.Kill()
	}
}

// SSH and managed compute deliberately exist as architectural boundaries. They
// return explicit errors until transport/control-plane implementations land.
type SSH struct{ ID string }

func (s SSH) Name() string                  { return s.ID }
func (s SSH) Kind() string                  { return "ssh" }
func (s SSH) Prepare(context.Context) error { return fmt.Errorf("SSH compute is not implemented") }
func (s SSH) Inspect(context.Context) (Hardware, error) {
	return Hardware{}, fmt.Errorf("SSH compute is not implemented")
}
func (s SSH) Execute(context.Context, Command) (Process, error) {
	return nil, fmt.Errorf("SSH compute is not implemented")
}

type Managed struct{}

func (Managed) Name() string { return "backpack" }
func (Managed) Kind() string { return "managed" }
func (Managed) Prepare(context.Context) error {
	return fmt.Errorf("Backpack managed compute has no control plane yet")
}
func (Managed) Inspect(context.Context) (Hardware, error) {
	return Hardware{}, fmt.Errorf("Backpack managed compute has no control plane yet")
}
func (Managed) Execute(context.Context, Command) (Process, error) {
	return nil, fmt.Errorf("Backpack managed compute has no control plane yet")
}
