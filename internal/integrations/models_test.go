package integrations

import (
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

func TestCodingModelsRequireExplicitCapability(t *testing.T) {
	models := []catalog.Model{
		{ID: "coder-by-name-only", Capabilities: []string{"chat"}},
		{ID: "ordinary-name", Capabilities: []string{"chat", "CODE"}, Aliases: []string{"original"}},
		{ID: "vision", Capabilities: []string{"vision"}},
	}
	filtered := FilterCodingModels(models)
	if len(filtered) != 1 || filtered[0].ID != "ordinary-name" {
		t.Fatalf("unexpected coding models: %#v", filtered)
	}
	filtered[0].Aliases[0] = "mutated"
	if models[1].Aliases[0] != "original" {
		t.Fatal("filter returned shared model slices")
	}
	descriptor := testDescriptor("agent")
	if ModelSupports(descriptor, models[0]) || !ModelSupports(descriptor, models[1]) {
		t.Fatal("descriptor capability check used something other than explicit capabilities")
	}
}

func TestContextRecommendations(t *testing.T) {
	tests := []struct {
		available, recommended int
		want                   ContextStatus
	}{{0, 32768, ContextUnknown}, {8192, 32768, ContextBelowRecommended}, {32768, 32768, ContextRecommended}, {65536, 32768, ContextRecommended}}
	for _, test := range tests {
		check, err := CheckContext(test.available, test.recommended)
		if err != nil || check.Status != test.want {
			t.Fatalf("CheckContext(%d, %d) = %#v, %v", test.available, test.recommended, check, err)
		}
	}
	if _, err := CheckContext(-1, 1); err == nil {
		t.Fatal("negative context accepted")
	}
	check, err := CheckRecommendedContext(testDescriptor("agent"), 8192)
	if err != nil || check.Status != ContextBelowRecommended || check.RecommendedTokens != 32768 {
		t.Fatalf("descriptor context check = %#v, %v", check, err)
	}
}
