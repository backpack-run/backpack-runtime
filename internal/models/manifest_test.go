package models

import (
	"strings"
	"testing"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestLegacyRuntimeContractNormalization(t *testing.T) {
	data := []byte(`schema_version: 1
model:
  id: qwen3-asr-0.6b
  display_name: Qwen3 ASR
upstream:
  repo: Qwen/Qwen3-ASR-0.6B
packages:
  - id: bf16
    format: safetensors
    precision: BF16
    filename: model.safetensors
    sha256: 79d6cbd4c98c7bbffe9db2edac07f56cd6637d0d5944b27f6c2b8353840323ea
    size_bytes: 1880560703
    runtime:
      provider: qwen-asr
      configured_revision: qwen-asr==0.0.6
      minimum_version: 0.0.6
`)
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	r := m.RuntimeFor(m.Packages[0])
	if r.Engine != "qwen-asr" || r.Version != "0.0.6" || r.Environment != "isolated-python" {
		t.Fatalf("unexpected contract: %#v", r)
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
runtime: {engine: custom-engine, version: "2.1", environment: isolated-python}
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

func TestProjectorIsARequiredTypedArtifact(t *testing.T) {
	manifest := []byte(`schema_version: 2
model: {id: vision-model, display_name: Vision}
upstream: {repo: org/vision}
packages:
  - id: gguf-q4-k-m
    format: gguf
    precision: Q4_K_M
    filename: model.gguf
    sha256: ` + testDigest + `
    size_bytes: 1
    runtime: {provider: llama.cpp}
    projector:
      id: mmproj-f16
      role: multimodal-projector
      format: gguf
      filename: mmproj.gguf
      sha256: ` + testDigest + `
      size_bytes: 2
`)
	m, err := ParseManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files := m.Packages[0].RequiredFiles()
	if len(files) != 2 || files[1].Role != "multimodal-projector" {
		t.Fatalf("unexpected required files: %#v", files)
	}
}
