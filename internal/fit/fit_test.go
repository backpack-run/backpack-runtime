package fit

import (
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

func TestFitClassificationAndRefusal(t *testing.T) {
	pkg := models.Package{SizeBytes: 20 << 30, Runtime: models.RuntimeInfo{Provider: "llama.cpp"}, Hardware: models.Hardware{EstimatedRAMGB: 24, RecommendedRAM: 32, EstimatedVRAM: 20}}
	if got := Evaluate(pkg, compute.Hardware{MemoryTotalGB: 16}); got.State != Unsupported || Refusal(got) == nil {
		t.Fatalf("low RAM report %#v", got)
	}
	if got := Evaluate(pkg, compute.Hardware{MemoryTotalGB: 64}); got.State != RemoteRecommended || Refusal(got) == nil {
		t.Fatalf("CPU fallback report %#v", got)
	}
	if got := Evaluate(pkg, compute.Hardware{MemoryTotalGB: 64, GPUs: []compute.GPU{{VRAMGB: 24}}}); got.State != Excellent || Refusal(got) != nil {
		t.Fatalf("GPU fit report %#v", got)
	}
}

func TestGLM53FlashIsRefusedBeforeDownloadOnConsumerHardware(t *testing.T) {
	pkg := models.Package{
		SizeBytes: 193813823776,
		Runtime:   models.RuntimeInfo{Provider: "llama.cpp"},
		Hardware:  models.Hardware{EstimatedRAMGB: 233.71, EstimatedVRAM: 214.33, RecommendedRAM: 263.78},
	}
	report := Evaluate(pkg, compute.Hardware{MemoryTotalGB: 32, GPUs: []compute.GPU{{VRAMGB: 8}}})
	if report.State != Unsupported || Refusal(report) == nil {
		t.Fatalf("large GLM package was not refused: %#v", report)
	}
}
