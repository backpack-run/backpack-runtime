package catalog

import "testing"

func TestBundledCatalogHasTwelvePublishedModels(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Models) != 12 {
		t.Fatalf("got %d models", len(c.Models))
	}
	m, err := c.Resolve("smollm2-135m")
	if err != nil {
		t.Fatal(err)
	}
	if m.RuntimeEngine != "llama.cpp" {
		t.Fatalf("unexpected runtime %q", m.RuntimeEngine)
	}
	qwen, err := c.Resolve("qwen3-coder-30b-a3b")
	if err != nil {
		t.Fatal(err)
	}
	if qwen.Status != "supported" || !containsCapability(qwen.Capabilities, "tool-calling") {
		t.Fatalf("Qwen qualification metadata = %#v", qwen)
	}
}

func containsCapability(capabilities []string, expected string) bool {
	for _, capability := range capabilities {
		if capability == expected {
			return true
		}
	}
	return false
}

func TestCatalogRejectsDuplicateAliasesAndRepositories(t *testing.T) {
	base := Model{ID: "one", Aliases: []string{"alias"}, Repository: "backpack-run/one", Revision: "0123456789012345678901234567890123456789", RuntimeEngine: "llama.cpp", Status: "experimental"}
	duplicateAlias := base
	duplicateAlias.ID, duplicateAlias.Repository, duplicateAlias.Aliases = "two", "backpack-run/two", []string{"ALIAS"}
	if err := (Catalog{SchemaVersion: 1, Models: []Model{base, duplicateAlias}}).Validate(); err == nil {
		t.Fatal("accepted duplicate alias")
	}
	duplicateRepository := base
	duplicateRepository.ID, duplicateRepository.Aliases = "two", []string{"two"}
	if err := (Catalog{SchemaVersion: 1, Models: []Model{base, duplicateRepository}}).Validate(); err == nil {
		t.Fatal("accepted duplicate repository")
	}
}
