package runtimebundle

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed catalog.v1.json
var builtinCatalog []byte

type Catalog struct {
	SchemaVersion  int       `json:"schema_version"`
	CatalogVersion string    `json:"catalog_version"`
	Runtimes       []Runtime `json:"runtimes"`
}

type Runtime struct {
	Engine              string     `json:"engine"`
	Version             string     `json:"version"`
	CompatibleRevisions []string   `json:"compatible_revisions,omitempty"`
	Source              Source     `json:"source"`
	License             string     `json:"license"`
	Capabilities        []string   `json:"capabilities"`
	LicenseArtifacts    []Artifact `json:"license_artifacts,omitempty"`
	Variants            []Variant  `json:"variants"`
}

type Source struct {
	Project     string `json:"project"`
	Repository  string `json:"repository"`
	Revision    string `json:"revision"`
	Attestation string `json:"attestation,omitempty"`
}

type Variant struct {
	ID           string     `json:"id"`
	OS           string     `json:"os"`
	Architecture string     `json:"architecture"`
	Backend      string     `json:"backend"`
	Executable   string     `json:"executable"`
	Artifacts    []Artifact `json:"artifacts"`
}

type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Format string `json:"format"`
	Path   string `json:"path,omitempty"`
}

func LoadCatalog() (Catalog, error) {
	data := builtinCatalog
	if path := os.Getenv("BACKPACK_RUNTIME_CATALOG"); path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return Catalog{}, fmt.Errorf("read runtime catalog: %w", err)
		}
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parse runtime catalog: %w", err)
	}
	if c.SchemaVersion != 1 || c.CatalogVersion == "" || len(c.Runtimes) == 0 {
		return c, fmt.Errorf("unsupported or empty runtime catalog")
	}
	for _, r := range c.Runtimes {
		if !validComponent(r.Engine) || !validComponent(r.Version) || r.Source.Repository == "" || len(r.Variants) == 0 {
			return c, fmt.Errorf("invalid runtime catalog entry for %q", r.Engine)
		}
		for _, v := range r.Variants {
			if !validComponent(v.ID) || v.OS == "" || v.Architecture == "" || v.Backend == "" || v.Executable == "" || len(v.Artifacts) == 0 {
				return c, fmt.Errorf("invalid variant in runtime %q", r.Engine)
			}
			for _, a := range v.Artifacts {
				if err := validateArtifact(a); err != nil {
					return c, fmt.Errorf("runtime artifact %q must use HTTPS and SHA-256", a.URL)
				}
			}
		}
		for _, a := range r.LicenseArtifacts {
			if err := validateArtifact(a); err != nil {
				return c, err
			}
		}
	}
	return c, nil
}

func validateArtifact(a Artifact) error {
	if !strings.HasPrefix(a.URL, "https://") || len(a.SHA256) != 64 {
		return fmt.Errorf("runtime artifact %q must use HTTPS and SHA-256", a.URL)
	}
	if a.Format == "file" && (a.Path == "" || strings.Contains(a.Path, "/") || strings.Contains(a.Path, "\\") || !validComponent(a.Path)) {
		return fmt.Errorf("runtime file artifact has unsafe path %q", a.Path)
	}
	return nil
}

func validComponent(v string) bool {
	if v == "" || v == "." || v == ".." {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}
