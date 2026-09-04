package runtime

import (
	"context"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"testing"
)

type fakeAdapter struct{ name string }

func (f fakeAdapter) Name() string                                                     { return f.name }
func (f fakeAdapter) Supports(r models.RuntimeRequirement) bool                        { return SameEngine(r.Engine, f.name) }
func (f fakeAdapter) Prepare(context.Context, *models.Installed, compute.Target) error { return nil }
func (f fakeAdapter) Start(context.Context, *models.Installed, compute.Target, StartOptions) (*Session, error) {
	return &Session{}, nil
}
func (f fakeAdapter) Health(context.Context, *Session) error { return nil }
func (f fakeAdapter) Stop(context.Context, *Session) error   { return nil }
func (f fakeAdapter) Capabilities() []string                 { return nil }
func TestRegistrySelectsByManifestEngine(t *testing.T) {
	r := NewRegistry(fakeAdapter{"kokoro"})
	a, err := r.Select(models.RuntimeRequirement{Engine: "KOKORO"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Name() != "kokoro" {
		t.Fatal("wrong adapter")
	}
	if _, err = r.Select(models.RuntimeRequirement{Engine: "qwen-asr"}); err == nil {
		t.Fatal("unsupported engine accepted")
	}
}
