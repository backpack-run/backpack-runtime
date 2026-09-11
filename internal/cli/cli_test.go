package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHumanBytes(t *testing.T) {
	for _, test := range []struct {
		bytes int64
		want  string
	}{{0, "-"}, {512 * 1024 * 1024, "512 MiB"}, {3 * 1024 * 1024 * 1024, "3.0 GiB"}} {
		if got := humanBytes(test.bytes); got != test.want {
			t.Fatalf("humanBytes(%d) = %q, want %q", test.bytes, got, test.want)
		}
	}
}

func TestConventionalVersionFlags(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	for _, argument := range []string{"version", "--version", "-v"} {
		var output bytes.Buffer
		if err := Run(context.Background(), []string{argument}, &output, &output, "v0.1.0-alpha.1"); err != nil {
			t.Fatalf("%s: %v", argument, err)
		}
		if !strings.Contains(output.String(), "backpack v0.1.0-alpha.1") {
			t.Fatalf("%s output %q", argument, output.String())
		}
	}
}

func TestModelsJSONIsMachineReadableAndComplete(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"models", "--json"}, &output, &output, "test"); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"catalog_version"`, `"smollm2-135m"`, `"installed": false`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("models JSON missing %s: %s", expected, output.String())
		}
	}
}

func TestDevelopmentBuildUpdateDiagnosticDoesNotContactNetwork(t *testing.T) {
	a := &app{version: "dev (commit unknown)"}
	status := a.updateDiagnostic(context.Background())
	if status.Status != "unknown" || status.Reason == "" {
		t.Fatalf("unexpected update diagnostic: %#v", status)
	}
}
