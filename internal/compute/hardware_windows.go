//go:build windows

package compute

import (
	"context"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type memoryStatusEx struct {
	Length, MemoryLoad                                                                                   uint32
	TotalPhys, AvailPhys, TotalPageFile, AvailPageFile, TotalVirtual, AvailVirtual, AvailExtendedVirtual uint64
}

func fillPlatformHardware(ctx context.Context, h *Hardware) {
	if h.CPU == "unknown" {
		if value := os.Getenv("PROCESSOR_IDENTIFIER"); value != "" {
			h.CPU = value
		}
	}
	if h.MemoryTotalGB == 0 {
		status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
		proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
		if ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&status))); ok != 0 {
			h.MemoryTotalGB = float64(status.TotalPhys) / 1073741824
		}
	}
	if h.DiskAvailableGB == 0 {
		cwd, _ := os.Getwd()
		path, _ := windows.UTF16PtrFromString(cwd)
		var free, total, totalFree uint64
		if windows.GetDiskFreeSpaceEx(path, &free, &total, &totalFree) == nil {
			h.DiskAvailableGB = float64(free) / 1073741824
		}
	}
	if len(h.GPUs) == 0 {
		out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total,driver_version", "--format=csv,noheader,nounits").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				parts := strings.Split(line, ",")
				if len(parts) < 2 {
					continue
				}
				mb, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				h.GPUs = append(h.GPUs, GPU{Vendor: "nvidia", Model: strings.TrimSpace(parts[0]), VRAMGB: mb / 1024})
				if !contains(h.Backends, "cuda") {
					h.Backends = append(h.Backends, "cuda", "vulkan")
				}
			}
		}
	}
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
