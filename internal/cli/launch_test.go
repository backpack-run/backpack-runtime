package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
)

func TestSelectCodingModelFiltersIncompatibleModels(t *testing.T) {
	models := []catalog.Model{
		{ID: "chat", Capabilities: []string{"text"}},
		{ID: "coder", Aliases: []string{"code"}, Capabilities: []string{"text", "coding", "tool-calling"}, Status: "experimental", Agents: map[string]catalog.AgentCompatibility{"codex": {Status: "compatible-experimental", Protocol: "responses"}}},
	}
	var output bytes.Buffer
	selected, err := selectCodingModel(strings.NewReader("1\n"), &output, models, "codex", true)
	if err != nil {
		t.Fatal(err)
	}
	if selected != "coder" || strings.Contains(output.String(), "chat          ") {
		t.Fatalf("selected=%q output=%q", selected, output.String())
	}
	if _, err = selectCodingModel(strings.NewReader(""), &output, models, "codex", false); err == nil || !strings.Contains(err.Error(), "--model is required") {
		t.Fatalf("unexpected non-interactive result: %v", err)
	}
}

func TestLaunchListAndHelp(t *testing.T) {
	t.Setenv("BACKPACK_HOME", t.TempDir())
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"launch", "list"}, &output, &output, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Claude Code") || !strings.Contains(output.String(), "Codex CLI") || !strings.Contains(output.String(), "Codex App") || !strings.Contains(output.String(), "OpenCode") || !strings.Contains(output.String(), "Pi") {
		t.Fatalf("unexpected launch list: %s", output.String())
	}
	output.Reset()
	if err := Run(context.Background(), []string{"launch", "--help"}, &output, &output, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "tool-args") || !strings.Contains(strings.ToLower(output.String()), "eligible open coding model") {
		t.Fatalf("unexpected launch help: %s", output.String())
	}
}
