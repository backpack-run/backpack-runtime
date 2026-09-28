package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

//go:embed catalog.v1.json
var bundled []byte

type Catalog struct {
	SchemaVersion  int     `json:"schema_version"`
	CatalogVersion string  `json:"catalog_version"`
	Models         []Model `json:"models"`
}
type Model struct {
	ID            string                        `json:"id"`
	Aliases       []string                      `json:"aliases"`
	ManifestIDs   []string                      `json:"manifest_ids,omitempty"`
	DisplayName   string                        `json:"display_name"`
	Repository    string                        `json:"repository"`
	Revision      string                        `json:"revision"`
	Capabilities  []string                      `json:"capabilities"`
	Protocols     []string                      `json:"protocols,omitempty"`
	ContextWindow int                           `json:"context_window,omitempty"`
	Visibility    string                        `json:"visibility,omitempty"`
	Agents        map[string]AgentCompatibility `json:"agent_compatibility,omitempty"`
	RuntimeEngine string                        `json:"runtime_engine"`
	Status        string                        `json:"status"`
}

// AgentCompatibility is trusted qualification evidence, not a model-authored
// claim. It records whether one exact packaged model is fit for an external
// coding-agent protocol.
type AgentCompatibility struct {
	Status   string `json:"status"`
	Protocol string `json:"protocol"`
	Reason   string `json:"reason,omitempty"`
}

func Load() (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(bundled, &c); err != nil {
		return c, fmt.Errorf("load catalog: %w", err)
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (c Catalog) Validate() error {
	if c.SchemaVersion != 1 || len(c.Models) == 0 {
		return fmt.Errorf("unsupported or empty catalog")
	}
	identities := map[string]string{}
	repositories := map[string]string{}
	for _, model := range c.Models {
		if model.ID == "" || model.Repository == "" || model.RuntimeEngine == "" || model.Status == "" || !revisionPattern.MatchString(model.Revision) {
			return fmt.Errorf("catalog model %q has incomplete trusted metadata", model.ID)
		}
		repository := strings.ToLower(model.Repository)
		if previous, exists := repositories[repository]; exists {
			return fmt.Errorf("catalog repository %q is duplicated by %q and %q", model.Repository, previous, model.ID)
		}
		repositories[repository] = model.ID
		if model.ContextWindow < 0 {
			return fmt.Errorf("catalog model %q has a negative context window", model.ID)
		}
		for _, protocol := range model.Protocols {
			if !contains([]string{"chat-completions", "responses", "messages"}, protocol) {
				return fmt.Errorf("catalog model %q declares unknown protocol %q", model.ID, protocol)
			}
		}
		for agent, compatibility := range model.Agents {
			if strings.TrimSpace(agent) == "" || !contains([]string{"qualified", "compatible-experimental", "untested", "incompatible"}, compatibility.Status) {
				return fmt.Errorf("catalog model %q has invalid agent compatibility for %q", model.ID, agent)
			}
			if compatibility.Status != "incompatible" && strings.TrimSpace(compatibility.Protocol) == "" {
				return fmt.Errorf("catalog model %q agent %q requires a protocol", model.ID, agent)
			}
			if compatibility.Protocol != "" && !contains(model.Protocols, compatibility.Protocol) {
				return fmt.Errorf("catalog model %q agent %q uses undeclared protocol %q", model.ID, agent, compatibility.Protocol)
			}
			if compatibility.Status == "qualified" || compatibility.Status == "compatible-experimental" {
				if !contains(model.Capabilities, "coding") || !contains(model.Capabilities, "tool-calling") || model.ContextWindow == 0 {
					return fmt.Errorf("catalog model %q cannot be agent-eligible without coding, tool-calling, and a trusted context window", model.ID)
				}
			}
		}
		for _, identity := range append([]string{model.ID}, model.Aliases...) {
			key := strings.ToLower(strings.TrimSpace(identity))
			if key == "" {
				return fmt.Errorf("catalog model %q has an empty alias", model.ID)
			}
			if previous, exists := identities[key]; exists && previous != model.ID {
				return fmt.Errorf("catalog identity %q is duplicated by %q and %q", identity, previous, model.ID)
			}
			identities[key] = model.ID
		}
	}
	return nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
func (c Catalog) Resolve(name string) (Model, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, m := range c.Models {
		if strings.ToLower(m.ID) == name || strings.ToLower(m.Repository) == name {
			return m, nil
		}
		for _, a := range m.Aliases {
			if strings.ToLower(a) == name {
				return m, nil
			}
		}
	}
	return Model{}, fmt.Errorf("unknown model %q; run `backpack models` to see official aliases", name)
}
