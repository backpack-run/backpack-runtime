// Package integrations contains the policy-free building blocks used to connect
// external agent CLIs to Backpack. It deliberately does not install or execute
// those CLIs.
package integrations

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Descriptor describes an external CLI without including provider credentials
// or user-specific configuration.
type Descriptor struct {
	ID                       string
	DisplayName              string
	ExecutableCandidates     []string
	RequiredModelCapability  string
	RecommendedContextTokens int
}

var descriptorID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func (d Descriptor) validate() error {
	if !descriptorID.MatchString(d.ID) {
		return fmt.Errorf("integration ID %q must be a lowercase slug", d.ID)
	}
	if strings.TrimSpace(d.DisplayName) == "" {
		return fmt.Errorf("integration %q requires a display name", d.ID)
	}
	if len(d.ExecutableCandidates) == 0 {
		return fmt.Errorf("integration %q requires at least one executable candidate", d.ID)
	}
	seen := make(map[string]struct{}, len(d.ExecutableCandidates))
	for _, candidate := range d.ExecutableCandidates {
		if candidate == "" || candidate != strings.TrimSpace(candidate) || strings.ContainsAny(candidate, `/\\`) {
			return fmt.Errorf("integration %q has invalid PATH executable candidate %q", d.ID, candidate)
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("integration %q repeats executable candidate %q", d.ID, candidate)
		}
		seen[key] = struct{}{}
	}
	if strings.TrimSpace(d.RequiredModelCapability) == "" {
		return fmt.Errorf("integration %q requires an explicit model capability", d.ID)
	}
	if d.RecommendedContextTokens < 0 {
		return fmt.Errorf("integration %q has a negative context recommendation", d.ID)
	}
	return nil
}

func cloneDescriptor(d Descriptor) Descriptor {
	d.ExecutableCandidates = append([]string(nil), d.ExecutableCandidates...)
	return d
}

// Registry owns immutable integration descriptors. It contains no installation
// fallback: an unavailable executable remains unavailable.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Descriptor
}

func NewRegistry(descriptors ...Descriptor) (*Registry, error) {
	r := &Registry{entries: make(map[string]Descriptor, len(descriptors))}
	for _, descriptor := range descriptors {
		if err := r.Register(descriptor); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(descriptor Descriptor) error {
	if r == nil {
		return fmt.Errorf("integration registry is nil")
	}
	if err := descriptor.validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = map[string]Descriptor{}
	}
	key := strings.ToLower(descriptor.ID)
	if _, exists := r.entries[key]; exists {
		return fmt.Errorf("integration %q is already registered", descriptor.ID)
	}
	r.entries[key] = cloneDescriptor(descriptor)
	return nil
}

func (r *Registry) Get(id string) (Descriptor, error) {
	if r == nil {
		return Descriptor{}, fmt.Errorf("integration registry is nil")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptor, ok := r.entries[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return Descriptor{}, fmt.Errorf("integration %q is not registered", id)
	}
	return cloneDescriptor(descriptor), nil
}

func (r *Registry) List() []Descriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Descriptor, 0, len(r.entries))
	for _, descriptor := range r.entries {
		items = append(items, cloneDescriptor(descriptor))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
