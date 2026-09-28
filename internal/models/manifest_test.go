package models

import (
	"strings"
	"testing"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestParseManifestRejectsUnknownFutureSchema(t *testing.T) {
	_, err := ParseManifest([]byte(`schema_version: 999
model:
  id: future
packages:
  - id: gguf
    format: gguf
    filename: model.gguf
    sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    size_bytes: 1
    runtime:
      provider: llama.cpp
`))
	if err == nil || !strings.Contains(err.Error(), "newer than supported version") {
		t.Fatalf("expected future schema rejection, got %v", err)
	}
}

func TestLegacyGGUFRuntimeUsesTestedRevisionAndManagedBundle(t *testing.T) {
	manifest := Manifest{Packages: []Package{{ID: "q4", Runtime: RuntimeInfo{Provider: "llama.cpp", TestedRevision: "abc123"}}}}
	r := manifest.RuntimeFor(manifest.Packages[0])
	if r.Version != "abc123" || r.Environment != "native-bundle" {
		t.Fatalf("runtime %#v", r)
	}
}

func TestExplicitRuntimeContractWins(t *testing.T) {
	data := []byte(`schema_version: 2
model: {id: future-model, display_name: Future}
upstream: {repo: org/model}
runtime: {engine: custom-engine, version: "2.1", environment: native-bundle}
packages:
  - id: weights
    format: safetensors
    precision: BF16
    filename: model.safetensors
    sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    size_bytes: 1
    runtime: {provider: legacy-name}
`)
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	r := m.RuntimeFor(m.Packages[0])
	if r.Engine != "custom-engine" || r.Version != "2.1" {
		t.Fatalf("explicit contract ignored: %#v", r)
	}
}

func TestRejectsTraversalArtifact(t *testing.T) {
	if within(`C:\models\one`, `C:\models\two\escape.gguf`) {
		t.Fatal("sibling path accepted")
	}
}

func TestSplitGGUFRequiresCompleteOrderedSet(t *testing.T) {
	manifest := []byte(`schema_version: 2
model: {id: split-model, display_name: Split}
upstream: {repo: org/split}
packages:
  - id: gguf-q4-k-m
    format: gguf
    precision: Q4_K_M
    filename: q4/model-00001-of-00003.gguf
    sha256: ` + testDigest + `
    size_bytes: 3
    runtime: {provider: llama.cpp}
    entrypoint: q4/model-00001-of-00003.gguf
    files:
      - {filename: q4/model-00001-of-00003.gguf, sha256: ` + testDigest + `, size_bytes: 1}
      - {filename: q4/model-00002-of-00003.gguf, sha256: ` + testDigest + `, size_bytes: 1}
`)
	_, err := ParseManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "missing shard 00003") {
		t.Fatalf("expected actionable missing-shard error, got %v", err)
	}
}

func TestManifestRejectsMediaAndProjectorContracts(t *testing.T) {
	for _, manifest := range []string{
		`schema_version: 1
model: {id: vision-model, input_modalities: [text, image], output_modalities: [text]}
packages:
  - {id: gguf, format: gguf, filename: model.gguf, sha256: ` + testDigest + `, size_bytes: 1, runtime: {provider: llama.cpp}}`,
		`schema_version: 1
model: {id: audio-model, input_modalities: [audio], output_modalities: [text]}
packages:
  - {id: gguf, format: gguf, filename: model.gguf, sha256: ` + testDigest + `, size_bytes: 1, runtime: {provider: llama.cpp}}`,
		`schema_version: 1
model: {id: projector-model}
packages:
  - id: gguf
    format: gguf
    filename: model.gguf
    sha256: ` + testDigest + `
    size_bytes: 1
    runtime: {provider: llama.cpp}
    projector: {filename: mmproj.gguf, sha256: ` + testDigest + `, size_bytes: 1}`,
		`schema_version: 2
model: {id: auxiliary-projector-model}
packages:
  - id: gguf
    format: gguf
    filename: model.gguf
    sha256: ` + testDigest + `
    size_bytes: 1
    runtime: {provider: llama.cpp}
    auxiliary_artifacts:
      - {id: projector, role: multimodal-projector, format: gguf, filename: mmproj.gguf, sha256: ` + testDigest + `, size_bytes: 1}`,
	} {
		_, err := ParseManifest([]byte(manifest))
		if err == nil || !strings.Contains(err.Error(), "coding-text scope") {
			t.Fatalf("expected media/projector rejection, got %v", err)
		}
	}
}
