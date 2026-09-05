package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
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
	ID            string   `json:"id"`
	Aliases       []string `json:"aliases"`
	ManifestIDs   []string `json:"manifest_ids,omitempty"`
	DisplayName   string   `json:"display_name"`
	Repository    string   `json:"repository"`
	Revision      string   `json:"revision"`
	Capabilities  []string `json:"capabilities"`
	RuntimeEngine string   `json:"runtime_engine"`
	Status        string   `json:"status"`
}

func Load() (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(bundled, &c); err != nil {
		return c, fmt.Errorf("load catalog: %w", err)
	}
	if c.SchemaVersion != 1 || len(c.Models) == 0 {
		return c, fmt.Errorf("unsupported or empty catalog")
	}
	return c, nil
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
