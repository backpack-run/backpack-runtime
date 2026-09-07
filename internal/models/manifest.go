package models

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const MaxSupportedSchemaVersion = 2

type Manifest struct {
	SchemaVersion   int                 `yaml:"schema_version" json:"schema_version"`
	Model           ModelInfo           `yaml:"model" json:"model"`
	Upstream        UpstreamInfo        `yaml:"upstream" json:"upstream"`
	Runtime         *RuntimeRequirement `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Packages        []Package           `yaml:"packages" json:"packages"`
	RuntimeServices []RuntimeService    `yaml:"runtime_services,omitempty" json:"runtime_services,omitempty"`
	Pipeline        *Pipeline           `yaml:"pipeline,omitempty" json:"pipeline,omitempty"`
}

type ModelInfo struct {
	ID               string   `yaml:"id" json:"id"`
	DisplayName      string   `yaml:"display_name" json:"display_name"`
	Architecture     string   `yaml:"architecture,omitempty" json:"architecture,omitempty"`
	ContextLength    int      `yaml:"context_length" json:"context_length"`
	Tasks            []string `yaml:"tasks,omitempty" json:"tasks,omitempty"`
	Capabilities     []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	InputModalities  []string `yaml:"input_modalities,omitempty" json:"input_modalities,omitempty"`
	OutputModalities []string `yaml:"output_modalities,omitempty" json:"output_modalities,omitempty"`
}

func (m *ModelInfo) UnmarshalYAML(node *yaml.Node) error {
	type raw struct {
		ID                  string `yaml:"id"`
		DisplayName         string `yaml:"display_name"`
		Architecture        string `yaml:"architecture"`
		ContextLength       int    `yaml:"context_length"`
		Tasks, Capabilities []string
		InputModalities     []string `yaml:"input_modalities"`
		OutputModalities    []string `yaml:"output_modalities"`
	}
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*m = ModelInfo{ID: r.ID, DisplayName: r.DisplayName, Architecture: r.Architecture, ContextLength: r.ContextLength, Tasks: r.Tasks, Capabilities: r.Capabilities, InputModalities: r.InputModalities, OutputModalities: r.OutputModalities}
	return nil
}

type UpstreamInfo struct {
	Repo     string      `yaml:"repo" json:"repo"`
	Revision string      `yaml:"revision" json:"revision"`
	License  LicenseInfo `yaml:"license" json:"license"`
}
type LicenseInfo struct {
	Identifier string `yaml:"identifier,omitempty" json:"identifier,omitempty"`
	URL        string `yaml:"url,omitempty" json:"url,omitempty"`
}

// RuntimeRequirement is the normalized packager/runtime handshake. New manifests
// should provide it explicitly. Legacy schema-v1 manifests are normalized from
// the selected package without consulting the model name.
type RuntimeRequirement struct {
	Engine      string `yaml:"engine" json:"engine"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`
	Environment string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Protocol    int    `yaml:"protocol_version,omitempty" json:"protocol_version,omitempty"`
}
type RuntimeInfo struct {
	Provider           string `yaml:"provider" json:"provider"`
	ConfiguredRevision string `yaml:"configured_revision,omitempty" json:"configured_revision,omitempty"`
	TestedRevision     string `yaml:"tested_revision,omitempty" json:"tested_revision,omitempty"`
	MinimumVersion     string `yaml:"minimum_version,omitempty" json:"minimum_version,omitempty"`
	Environment        string `yaml:"environment,omitempty" json:"environment,omitempty"`
}
type File struct {
	Filename  string `yaml:"filename" json:"filename"`
	SHA256    string `yaml:"sha256" json:"sha256"`
	SizeBytes int64  `yaml:"size_bytes" json:"size_bytes"`
	Role      string `yaml:"role,omitempty" json:"role,omitempty"`
}

// AuxiliaryArtifact is a package-bound runtime input. It deliberately carries
// data, not command-line fragments: manifests may select trusted adapter
// behavior, but can never inject executable arguments.
type AuxiliaryArtifact struct {
	ID        string      `yaml:"id" json:"id"`
	Role      string      `yaml:"role" json:"role"`
	Format    string      `yaml:"format" json:"format"`
	Precision string      `yaml:"precision,omitempty" json:"precision,omitempty"`
	Filename  string      `yaml:"filename" json:"filename"`
	SHA256    string      `yaml:"sha256" json:"sha256"`
	SizeBytes int64       `yaml:"size_bytes" json:"size_bytes"`
	Runtime   RuntimeInfo `yaml:"runtime,omitempty" json:"runtime,omitempty"`
}
type Hardware struct {
	EstimatedRAMGB float64 `yaml:"estimated_ram_gb" json:"estimated_ram_gb"`
	EstimatedVRAM  float64 `yaml:"estimated_vram_gb" json:"estimated_vram_gb"`
	RecommendedRAM float64 `yaml:"recommended_ram_gb" json:"recommended_ram_gb"`
	Note           string  `yaml:"note,omitempty" json:"note,omitempty"`
}
type Validation struct {
	Integrity string `yaml:"integrity" json:"integrity"`
	Load      string `yaml:"load" json:"load"`
	Inference string `yaml:"inference" json:"inference"`
}
type Package struct {
	ID         string      `yaml:"id" json:"id"`
	Format     string      `yaml:"format" json:"format"`
	Precision  string      `yaml:"precision" json:"precision"`
	Filename   string      `yaml:"filename" json:"filename"`
	SHA256     string      `yaml:"sha256" json:"sha256"`
	SizeBytes  int64       `yaml:"size_bytes" json:"size_bytes"`
	Runtime    RuntimeInfo `yaml:"runtime" json:"runtime"`
	Hardware   Hardware    `yaml:"hardware" json:"hardware"`
	Validation Validation  `yaml:"validation" json:"validation"`
	Files      []File      `yaml:"files,omitempty" json:"files,omitempty"`
	Entrypoint string      `yaml:"entrypoint,omitempty" json:"entrypoint,omitempty"`
	// Projector is retained for the current packager schema. New package
	// producers may use AuxiliaryArtifacts for projectors, tokenizers, and other
	// explicitly typed inputs.
	Projector          *AuxiliaryArtifact  `yaml:"projector,omitempty" json:"projector,omitempty"`
	AuxiliaryArtifacts []AuxiliaryArtifact `yaml:"auxiliary_artifacts,omitempty" json:"auxiliary_artifacts,omitempty"`
}

func (p Package) ArtifactFiles() []File {
	if len(p.Files) > 0 {
		return p.Files
	}
	return []File{{Filename: p.Filename, SHA256: p.SHA256, SizeBytes: p.SizeBytes}}
}

func (p Package) RequiredFiles() []File {
	files := append([]File(nil), p.ArtifactFiles()...)
	if p.Projector != nil {
		files = append(files, File{Filename: p.Projector.Filename, SHA256: p.Projector.SHA256, SizeBytes: p.Projector.SizeBytes, Role: p.Projector.Role})
	}
	for _, artifact := range p.AuxiliaryArtifacts {
		files = append(files, File{Filename: artifact.Filename, SHA256: artifact.SHA256, SizeBytes: artifact.SizeBytes, Role: artifact.Role})
	}
	return files
}

func (p Package) ArtifactByRole(role string) (File, bool) {
	for _, file := range p.RequiredFiles() {
		if strings.EqualFold(file.Role, role) {
			return file, true
		}
	}
	return File{}, false
}
func (p Package) RuntimeEntrypoint() string {
	if p.Entrypoint != "" {
		return p.Entrypoint
	}
	return p.Filename
}

type RuntimeService struct {
	ID              string   `yaml:"id" json:"id"`
	Version         string   `yaml:"version" json:"version"`
	ProtocolVersion int      `yaml:"protocol_version" json:"protocol_version"`
	Platform        string   `yaml:"platform" json:"platform"`
	Backend         string   `yaml:"backend" json:"backend"`
	Capabilities    []string `yaml:"capabilities" json:"capabilities"`
	Entrypoint      string   `yaml:"entrypoint" json:"entrypoint"`
	Files           []File   `yaml:"files" json:"files"`
}
type Pipeline struct {
	Engine        string `yaml:"engine" json:"engine"`
	Version       string `yaml:"version" json:"version"`
	PipelineClass string `yaml:"pipeline_class" json:"pipeline_class"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse backpack manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
func (m Manifest) Validate() error {
	if m.SchemaVersion < 1 {
		return errors.New("manifest schema_version must be at least 1")
	}
	if m.SchemaVersion > MaxSupportedSchemaVersion {
		return fmt.Errorf("manifest schema_version %d is newer than supported version %d", m.SchemaVersion, MaxSupportedSchemaVersion)
	}
	if strings.TrimSpace(m.Model.ID) == "" {
		return errors.New("manifest model.id is required")
	}
	if len(m.Packages) == 0 && m.Pipeline == nil {
		return errors.New("manifest must contain packages or a pipeline")
	}
	for _, p := range m.Packages {
		if p.ID == "" || (p.Runtime.Provider == "" && (m.Runtime == nil || m.Runtime.Engine == "")) {
			return fmt.Errorf("package %q must declare id and runtime.provider", p.ID)
		}
		for _, f := range p.RequiredFiles() {
			if f.Filename == "" || !validSHA256(f.SHA256) || f.SizeBytes <= 0 {
				return fmt.Errorf("package %q has invalid artifact metadata for %q", p.ID, f.Filename)
			}
		}
		if err := validatePackageArtifacts(p); err != nil {
			return fmt.Errorf("package %q: %w", p.ID, err)
		}
	}
	return nil
}

var splitGGUF = regexp.MustCompile(`(?i)^(.*)-(\d{5})-of-(\d{5})\.gguf$`)
var windowsAbsolutePath = regexp.MustCompile(`^[A-Za-z]:`)

func validatePackageArtifacts(p Package) error {
	seen := map[string]bool{}
	for _, f := range p.RequiredFiles() {
		name := strings.ReplaceAll(f.Filename, "\\", "/")
		if path.IsAbs(name) || strings.Contains(name, "../") || name == ".." || windowsAbsolutePath.MatchString(name) {
			return fmt.Errorf("unsafe artifact path %q", f.Filename)
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("duplicate artifact path %q", f.Filename)
		}
		seen[strings.ToLower(name)] = true
	}
	entry := strings.ReplaceAll(p.RuntimeEntrypoint(), "\\", "/")
	if !seen[strings.ToLower(entry)] {
		return fmt.Errorf("entrypoint %q is not in the required artifact set", entry)
	}
	match := splitGGUF.FindStringSubmatch(entry)
	if len(match) == 0 {
		return nil
	}
	if match[2] != "00001" {
		return fmt.Errorf("split GGUF entrypoint must be shard 00001, got %q", entry)
	}
	var total int
	if _, err := fmt.Sscanf(match[3], "%d", &total); err != nil || total < 2 {
		return fmt.Errorf("invalid split GGUF shard count in %q", entry)
	}
	for shard := 1; shard <= total; shard++ {
		required := fmt.Sprintf("%s-%05d-of-%05d.gguf", match[1], shard, total)
		if !seen[strings.ToLower(required)] {
			return fmt.Errorf("split GGUF is incomplete: missing shard %05d of %05d (%s)", shard, total, required)
		}
	}
	return nil
}

var exactVersion = regexp.MustCompile(`(?:^|[; ])[^=; ]+==([^; ]+)`)

func (m Manifest) RuntimeFor(p Package) RuntimeRequirement {
	if m.Runtime != nil && m.Runtime.Engine != "" {
		return *m.Runtime
	}
	v := p.Runtime.MinimumVersion
	if v == "" {
		if x := exactVersion.FindStringSubmatch(p.Runtime.ConfiguredRevision); len(x) == 2 {
			v = x[1]
		}
	}
	if v == "" {
		v = p.Runtime.TestedRevision
	}
	e := p.Runtime.Environment
	if e == "" {
		switch strings.ToLower(p.Runtime.Provider) {
		case "qwen-asr", "kokoro", "diffusers":
			e = "isolated-python"
		default:
			e = "native-bundle"
		}
	}
	return RuntimeRequirement{Engine: p.Runtime.Provider, Version: v, Environment: e}
}
func validSHA256(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range strings.ToLower(v) {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
