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

func TestModelsJSONIsMachineReadableAndHidesTestFixturesByDefault(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"models", "--json"}, &output, &output, "test"); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"catalog_version"`, `"qwen3-coder-30b-a3b"`, `"compatible_agents"`, `"installed": false`} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("models JSON missing %s: %s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "smollm2-135m") {
		t.Fatalf("default catalog exposed internal test fixture: %s", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), []string{"models", "--json", "--all"}, &output, &output, "test"); err != nil || !strings.Contains(output.String(), "smollm2-135m") {
		t.Fatalf("--all did not expose test fixture: %v %s", err, output.String())
	}
}

func TestRemovedTopLevelCommandsStayRemoved(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	for _, command := range []string{"launch", "model", "catalog", "list", "inspect", "hardware"} {
		var output bytes.Buffer
		err := Run(context.Background(), []string{command}, &output, &output, "test")
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("%s unexpectedly remained public: %v %s", command, err, output.String())
		}
	}
}

func TestModelsConsolidatesOperationalSubcommands(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"models", "installed"}, &output, &output, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "No models installed") {
		t.Fatalf("unexpected installed-model output: %s", output.String())
	}
}

func TestDevelopmentBuildUpdateDiagnosticDoesNotContactNetwork(t *testing.T) {
	a := &app{version: "dev (commit unknown)"}
	status := a.updateDiagnostic(context.Background())
	if status.Status != "unknown" || status.Reason == "" {
		t.Fatalf("unexpected update diagnostic: %#v", status)
	}
}

func TestCloudStatusAndDoctorNeverPrintAPIKey(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	t.Setenv("BACKPACK_API_KEY", "test-secret-must-not-appear")
	for _, arguments := range [][]string{{"cloud", "status", "--json"}, {"doctor", "--json"}} {
		var output bytes.Buffer
		if err := Run(context.Background(), arguments, &output, &output, "dev"); err != nil {
			t.Fatalf("%v: %v", arguments, err)
		}
		if strings.Contains(output.String(), "test-secret-must-not-appear") {
			t.Fatalf("%v leaked the Cloud API key: %s", arguments, output.String())
		}
		if !strings.Contains(output.String(), "api-key-environment") && arguments[0] == "cloud" {
			t.Fatalf("cloud status omitted the credential source: %s", output.String())
		}
	}
}
