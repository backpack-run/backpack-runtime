package models

import "testing"

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
