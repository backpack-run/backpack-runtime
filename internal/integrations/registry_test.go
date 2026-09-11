package integrations

import (
	"reflect"
	"testing"
)

func testDescriptor(id string) Descriptor {
	return Descriptor{ID: id, DisplayName: "Test Agent", ExecutableCandidates: []string{id, id + ".exe"}, RequiredModelCapability: "code", RecommendedContextTokens: 32768}
}

func TestRegistryValidatesAndReturnsDefensiveCopies(t *testing.T) {
	r, err := NewRegistry(testDescriptor("bravo"), testDescriptor("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{r.List()[0].ID, r.List()[1].ID}; !reflect.DeepEqual(got, []string{"alpha", "bravo"}) {
		t.Fatalf("unexpected registry order: %v", got)
	}
	descriptor, err := r.Get("ALPHA")
	if err != nil {
		t.Fatal(err)
	}
	descriptor.ExecutableCandidates[0] = "mutated"
	again, _ := r.Get("alpha")
	if again.ExecutableCandidates[0] == "mutated" {
		t.Fatal("registry descriptor was mutated through returned slice")
	}
	if err = r.Register(testDescriptor("alpha")); err == nil {
		t.Fatal("duplicate integration accepted")
	}
}

func TestDescriptorValidation(t *testing.T) {
	tests := []Descriptor{
		{},
		{ID: "Bad", DisplayName: "Bad", ExecutableCandidates: []string{"bad"}, RequiredModelCapability: "code"},
		{ID: "bad", DisplayName: "", ExecutableCandidates: []string{"bad"}, RequiredModelCapability: "code"},
		{ID: "bad", DisplayName: "Bad", ExecutableCandidates: []string{`tools/bad`}, RequiredModelCapability: "code"},
		{ID: "bad", DisplayName: "Bad", ExecutableCandidates: []string{"bad", "BAD"}, RequiredModelCapability: "code"},
		{ID: "bad", DisplayName: "Bad", ExecutableCandidates: []string{"bad"}},
		{ID: "bad", DisplayName: "Bad", ExecutableCandidates: []string{"bad"}, RequiredModelCapability: "code", RecommendedContextTokens: -1},
	}
	for _, descriptor := range tests {
		if _, err := NewRegistry(descriptor); err == nil {
			t.Fatalf("invalid descriptor accepted: %#v", descriptor)
		}
	}
}
