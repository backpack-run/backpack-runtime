package fit

import (
	"fmt"
	"math"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

type State string

const (
	Excellent         State = "excellent"
	Good              State = "good"
	Constrained       State = "constrained"
	RemoteRecommended State = "remote-recommended"
	Unsupported       State = "unsupported"
)

type Report struct {
	State          State   `json:"state"`
	RequiredRAMGB  float64 `json:"required_ram_gb"`
	RecommendedRAM float64 `json:"recommended_ram_gb"`
	RequiredVRAMGB float64 `json:"required_vram_gb,omitempty"`
	SystemRAMGB    float64 `json:"system_ram_gb,omitempty"`
	DetectedVRAMGB float64 `json:"detected_vram_gb,omitempty"`
	Reason         string  `json:"reason"`
}

func Evaluate(pkg models.Package, hardware compute.Hardware) Report {
	artifactGB := float64(pkg.SizeBytes) / (1024 * 1024 * 1024)
	requiredRAM := math.Max(pkg.Hardware.EstimatedRAMGB, artifactGB*1.15)
	recommendedRAM := math.Max(pkg.Hardware.RecommendedRAM, requiredRAM*1.25)
	availableVRAM := 0.0
	for _, gpu := range hardware.GPUs {
		availableVRAM = math.Max(availableVRAM, gpu.VRAMGB)
	}
	report := Report{State: Good, RequiredRAMGB: requiredRAM, RecommendedRAM: recommendedRAM, RequiredVRAMGB: pkg.Hardware.EstimatedVRAM, SystemRAMGB: hardware.MemoryTotalGB, DetectedVRAMGB: availableVRAM, Reason: "model fits detected system memory"}
	if hardware.MemoryTotalGB > 0 && requiredRAM > hardware.MemoryTotalGB*0.9 {
		report.State = Unsupported
		report.Reason = "estimated model memory exceeds safely available system RAM"
		return report
	}
	if pkg.Hardware.EstimatedVRAM > 0 && availableVRAM >= pkg.Hardware.EstimatedVRAM {
		report.State = Excellent
		report.Reason = "model fits detected accelerator memory"
		return report
	}
	if pkg.Hardware.EstimatedVRAM >= 8 && availableVRAM < pkg.Hardware.EstimatedVRAM && hasAcceleratorRequirement(pkg.Runtime.Provider) {
		report.State = RemoteRecommended
		report.Reason = "the model does not fit detected accelerator memory; CPU fallback is likely impractical"
		return report
	}
	if hardware.MemoryTotalGB > 0 && recommendedRAM > hardware.MemoryTotalGB*0.8 {
		report.State = Constrained
		report.Reason = "model may fit, but leaves little memory for the operating system and context"
	}
	return report
}

func Refusal(report Report) error {
	if report.State != Unsupported && report.State != RemoteRecommended {
		return nil
	}
	return fmt.Errorf("model fit is %s: %s (system RAM %.1f GB / %.1f GB estimated, detected VRAM %.1f GB / %.1f GB estimated); select remote compute or use --force", report.State, report.Reason, report.SystemRAMGB, report.RequiredRAMGB, report.DetectedVRAMGB, report.RequiredVRAMGB)
}

func hasAcceleratorRequirement(engine string) bool {
	switch strings.ToLower(engine) {
	case "llama.cpp", "diffusers":
		return true
	default:
		return false
	}
}
