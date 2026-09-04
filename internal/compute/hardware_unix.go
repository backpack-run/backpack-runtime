//go:build !windows

package compute

import (
	"context"
	"os/exec"
	"runtime"
)

func fillPlatformHardware(_ context.Context, h *Hardware) {
	if runtime.GOOS == "darwin" {
		h.Backends = append(h.Backends, "metal")
		return
	}
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		h.Backends = append(h.Backends, "cuda")
	}
	if _, err := exec.LookPath("vulkaninfo"); err == nil {
		h.Backends = append(h.Backends, "vulkan")
	}
}
