package catalog

import "testing"

func TestBundledCatalogHasElevenPublishedModels(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Models) != 11 {
		t.Fatalf("got %d models", len(c.Models))
	}
	m, err := c.Resolve("smollm2-135m")
	if err != nil {
		t.Fatal(err)
	}
	if m.RuntimeEngine != "llama.cpp" {
		t.Fatalf("unexpected runtime %q", m.RuntimeEngine)
	}
}
