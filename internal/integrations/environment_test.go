package integrations

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestEnvironmentOverlayApplyAndRedaction(t *testing.T) {
	token, _ := Secret("AGENT_TOKEN", "top-secret")
	endpoint, _ := Set("AGENT_ENDPOINT", "http://127.0.0.1:11434")
	remove, _ := Unset("REMOVE_ME")
	overlay, err := NewEnvironmentOverlay(token, endpoint, remove)
	if err != nil {
		t.Fatal(err)
	}
	baseToken := "AGENT_TOKEN=old"
	if runtime.GOOS == "windows" {
		baseToken = "agent_token=old"
	}
	applied := overlay.Apply([]string{"KEEP=yes", baseToken, "REMOVE_ME=x"})
	for _, want := range []string{"KEEP=yes", "AGENT_TOKEN=top-secret", "AGENT_ENDPOINT=http://127.0.0.1:11434"} {
		if !slices.Contains(applied, want) {
			t.Fatalf("missing %q in %v", want, applied)
		}
	}
	if slices.Contains(applied, baseToken) || slices.Contains(applied, "REMOVE_ME=x") {
		t.Fatalf("base value was not replaced or removed: %v", applied)
	}
	for _, formatted := range []string{overlay.String(), fmt.Sprintf("%v", overlay), fmt.Sprintf("%#v", overlay), fmt.Sprintf("%v", token), fmt.Sprintf("%#v", token)} {
		if strings.Contains(formatted, "top-secret") || !strings.Contains(formatted, redacted) {
			t.Fatalf("credential leaked or was not redacted: %q", formatted)
		}
	}
}

func TestEnvironmentValidation(t *testing.T) {
	if _, err := Secret("BAD=NAME", "x"); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err := Set("OK", "bad\x00value"); err == nil {
		t.Fatal("NUL value accepted")
	}
	a, _ := Set("SAME", "a")
	b, _ := Set("SAME", "b")
	if _, err := NewEnvironmentOverlay(a, b); err == nil {
		t.Fatal("duplicate overlay accepted")
	}
}
