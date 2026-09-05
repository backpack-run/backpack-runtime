package pythonruntime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTrustedLocksContainHashesForEveryDistribution(t *testing.T) {
	for name, definition := range definitions {
		data, err := assets.ReadFile("requirements/" + definition.Lock)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		distributions := parseLockedDistributions(data)
		if len(distributions) == 0 {
			t.Fatalf("%s lock is empty", name)
		}
		for _, distribution := range distributions {
			if len(distribution.Hashes) == 0 {
				t.Fatalf("%s: %s has no trusted hash", name, distribution.Requirement)
			}
		}
	}
}

func TestEnvironmentReuseRejectsChangedTrustedMetadata(t *testing.T) {
	definition := definitions["qwen-asr"]
	root := t.TempDir()
	python := filepath.Join(root, "environment", "Scripts", "python.exe")
	worker := filepath.Join(root, definition.Worker)
	if err := os.MkdirAll(filepath.Dir(python), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(python, []byte("python"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(worker, []byte("worker"), 0600); err != nil {
		t.Fatal(err)
	}
	requirements, _ := assets.ReadFile("requirements/" + definition.Lock)
	workerData, _ := assets.ReadFile("workers/" + definition.Worker)
	data, err := json.Marshal(environmentManifest(definition, requirements, workerData))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "environment.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if !validEnvironment(root, python, worker, definition) {
		t.Fatal("expected matching environment to be reusable")
	}
	definition.Index = "https://invalid.example/simple"
	if validEnvironment(root, python, worker, definition) {
		t.Fatal("environment was reused after trusted source metadata changed")
	}
}
