package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/models"
)

type StartOptions struct {
	Host                         string
	Port, ContextSize, GPULayers int
}
type Session struct {
	ID, ModelID, Runtime, Compute, Endpoint, Status string
	PID                                             int
	Process                                         compute.Process
}
type Adapter interface {
	Name() string
	Supports(models.RuntimeRequirement) bool
	Prepare(context.Context, *models.Installed, compute.Target) error
	Start(context.Context, *models.Installed, compute.Target, StartOptions) (*Session, error)
	Health(context.Context, *Session) error
	Stop(context.Context, *Session) error
	Capabilities() []string
}
type Registry struct{ adapters []Adapter }

func NewRegistry(adapters ...Adapter) *Registry { return &Registry{adapters} }
func (r *Registry) Select(requirement models.RuntimeRequirement) (Adapter, error) {
	for _, a := range r.adapters {
		if a.Supports(requirement) {
			return a, nil
		}
	}
	return nil, fmt.Errorf("runtime engine %q (%s) is declared by the package but no installed adapter supports it", requirement.Engine, requirement.Environment)
}
func SameEngine(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
