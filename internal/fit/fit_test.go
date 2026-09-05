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
