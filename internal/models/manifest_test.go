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

func TestGLM53FlashPublishedContractIsSingleGGUFWithProjector(t *testing.T) {
	manifest := []byte(`schema_version: 1
model:
  id: glm-5-3-flash
  architecture: Glm5NextForConditionalGeneration
  tasks: [image-text-to-text]
  input_modalities: [text, image]
  output_modalities: [text]
upstream:
  repo: zai-org/GLM-5.3-Flash
  revision: 690b705278a3a58e538fcb37c2ca8b5f9511213c
packages:
  - id: gguf-q4-k-m
    format: gguf
    precision: Q4_K_M
    filename: GLM-5.3-Flash-Q4_K_M.gguf
    sha256: 3e1f1720e869d98acd55a8f94b5efd78814a6ba0a2c2e4e609d637e9cca60406
    size_bytes: 193813823776
    runtime:
      provider: llama.cpp
      tested_revision: 8134115f88ed8018474e7db69afcfe97fb097fc4
    hardware:
      estimated_ram_gb: 233.71
      estimated_vram_gb: 214.33
      recommended_ram_gb: 263.78
    projector:
      id: mmproj-f16
      role: multimodal-projector
      format: gguf
      precision: F16
      filename: GLM-5.3-Flash-mmproj-F16.gguf
      sha256: f64a2e935c899224054258d2372d9ad4c19b760141044292fa6ee0fb5ff36624
      size_bytes: 1128047104
`)
	m, err := ParseManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	pkg := m.Packages[0]
	files := pkg.RequiredFiles()
	if len(pkg.ArtifactFiles()) != 1 || len(files) != 2 || files[1].Role != "multimodal-projector" {
		t.Fatalf("unexpected GLM artifact contract: %#v", files)
	}
	runtime := m.RuntimeFor(pkg)
	if runtime.Engine != "llama.cpp" || runtime.Version != "8134115f88ed8018474e7db69afcfe97fb097fc4" {
		t.Fatalf("unexpected GLM runtime contract: %#v", runtime)
	}
}
