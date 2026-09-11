package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/daemon"
	"github.com/backpack-run/backpack-runtime/internal/integrations"
	clientapi "github.com/backpack-run/backpack-runtime/pkg/client"
)

func (a *app) launchCommand(ctx context.Context, args []string) error {
	registry, err := integrations.Builtins()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack launch <list|doctor|claude|codex|opencode> [flags] [-- tool-args]")
	}
	if args[0] == "list" {
		return a.launchList(registry)
	}
	if args[0] == "doctor" {
		if len(args) < 2 {
			return fmt.Errorf("usage: backpack launch doctor <claude|codex|opencode> [--model model] [--compute target] [--json]")
		}
		return a.launchDoctor(ctx, registry, args[1], args[2:])
	}
	descriptor, err := registry.Get(args[0])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("launch "+descriptor.ID, flag.ContinueOnError)
	fs.SetOutput(a.err)
	modelName := fs.String("model", "", "Backpack coding model")
	computeName := fs.String("compute", "local", "Backpack compute target")
	contextTokens := fs.Int("context", 0, "context tokens; defaults to the integration recommendation bounded by the model maximum")
	keepAlive := fs.Bool("keep-alive", false, "leave the Backpack model session loaded after the external tool exits")
	force := fs.Bool("force", false, "run even when model fit recommends remote compute")
	if err = fs.Parse(args[1:]); err != nil {
		return err
	}
	if *modelName == "" {
		selected, selectErr := selectCodingModel(os.Stdin, a.out, a.catalog.Models, stdinIsTerminal())
		if selectErr != nil {
			return selectErr
		}
		*modelName = selected
	}
	entry, err := a.catalog.Resolve(*modelName)
	if err != nil {
		return err
	}
	if !integrations.ModelSupports(descriptor, entry) {
		return fmt.Errorf("model %q does not declare the required %q capability", entry.ID, descriptor.RequiredModelCapability)
	}
	resolved, err := a.models.ResolvePackage(ctx, entry)
	if err != nil {
		return err
	}
	modelContext := resolved.Manifest.Model.ContextLength
	desiredContext := *contextTokens
	if desiredContext == 0 {
		desiredContext = descriptor.RecommendedContextTokens
		if modelContext > 0 && modelContext < desiredContext {
			desiredContext = modelContext
		}
	}
	if modelContext > 0 && desiredContext > modelContext {
		return fmt.Errorf("requested context %d exceeds model maximum %d", desiredContext, modelContext)
	}
	contextCheck, _ := integrations.CheckRecommendedContext(descriptor, modelContext)
	if contextCheck.Status != integrations.ContextRecommended {
		fmt.Fprintf(a.err, "Warning: %s\n", contextCheck.Reason)
	}
	if !hasCatalogCapability(entry, "tool-calling") {
		fmt.Fprintf(a.err, "Warning: %s is not execution-qualified for agent tool calling; this launch path is experimental.\n", entry.ID)
	}
	installation, err := integrations.NewDiscovery().Detect(descriptor)
	if err != nil {
		if errors.Is(err, integrations.ErrExecutableNotFound) {
			return fmt.Errorf("%w\n%s", err, integrationInstallInstructions(descriptor.ID))
		}
		return err
	}
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Loading %s on compute target %s...\n", entry.ID, *computeName)
	stopEvents := a.watchEvents(ctx, api)
	defer stopEvents()
	session, err := api.CreateSession(ctx, clientapi.CreateSessionRequest{Model: entry.ID, Compute: *computeName, Options: clientapi.SessionOptions{ContextLength: desiredContext, GPULayers: "auto", Force: *force}})
	if err != nil {
		return err
	}
	if !*keepAlive {
		defer func() {
			stopContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = api.StopSession(stopContext, session.ID)
		}()
	}
	configRoot, err := filepath.Abs(filepath.Join(a.paths.Config, "integrations"))
	if err != nil {
		return err
	}
	configDirectory, err := integrations.IsolatedConfigDirectory(configRoot, descriptor.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(configDirectory, 0700); err != nil {
		return err
	}
	options := integrations.ProviderOptions{Endpoint: api.BaseURL, Model: entry.ID, ContextTokens: desiredContext, ConfigDirectory: configDirectory, Executable: installation.Executable, Passthrough: fs.Args()}
	var invocation integrations.Invocation
	switch descriptor.ID {
	case "claude":
		invocation, err = integrations.ClaudeInvocation(options)
	case "codex":
		options.CatalogPath = filepath.Join(configDirectory, "models.json")
		if err = integrations.WriteCodexModelCatalog(integrations.CodexCatalogOptions{Model: entry, ContextTokens: desiredContext, Path: options.CatalogPath}); err == nil {
			invocation, err = integrations.CodexInvocation(options)
		}
	case "opencode":
		invocation, err = integrations.OpenCodeInvocation(options)
	default:
		err = fmt.Errorf("integration %q has no launch builder", descriptor.ID)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Launching %s through Backpack at %s (session %s).\n", descriptor.DisplayName, api.BaseURL, session.ID)
	return integrations.Run(ctx, invocation, integrations.ProcessIO{Stdin: os.Stdin, Stdout: a.out, Stderr: a.err})
}

func (a *app) launchList(registry *integrations.Registry) error {
	fmt.Fprintln(a.out, "INTEGRATION  TOOL         STATUS")
	discovery := integrations.NewDiscovery()
	for _, descriptor := range registry.List() {
		installation, err := discovery.Detect(descriptor)
		status := "not installed"
		if err == nil {
			status = installation.Executable
		}
		fmt.Fprintf(a.out, "%-12s %-12s %s\n", descriptor.ID, descriptor.DisplayName, status)
	}
	return nil
}

func (a *app) launchDoctor(ctx context.Context, registry *integrations.Registry, integrationID string, args []string) error {
	descriptor, err := registry.Get(integrationID)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("launch doctor", flag.ContinueOnError)
	fs.SetOutput(a.err)
	modelName := fs.String("model", "", "Backpack coding model")
	computeName := fs.String("compute", "local", "Backpack compute target")
	asJSON := fs.Bool("json", false, "print machine-readable diagnostics")
	if err = fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack launch doctor %s [--model model] [--compute target] [--json]", integrationID)
	}
	report := map[string]any{"integration": descriptor.ID, "display_name": descriptor.DisplayName, "required_api": map[string]string{"claude": "Anthropic Messages /v1/messages", "codex": "OpenAI Responses /v1/responses", "opencode": "OpenAI Chat Completions /v1/chat/completions"}[descriptor.ID], "compute": *computeName, "agent_qualified": false}
	if installation, detectErr := integrations.NewDiscovery().Detect(descriptor); detectErr == nil {
		report["installed"] = true
		report["executable"] = installation.Executable
		if version, versionErr := integrations.Version(ctx, installation); versionErr == nil {
			report["version"] = version
		} else {
			report["version_error"] = versionErr.Error()
		}
	} else {
		report["installed"] = false
		report["install_instructions"] = integrationInstallInstructions(descriptor.ID)
	}
	if *computeName == "local" {
		report["compute_configured"] = true
	} else if _, targetErr := a.targetStore().Get(*computeName); targetErr == nil {
		report["compute_configured"] = true
	} else {
		report["compute_configured"] = false
		report["compute_error"] = targetErr.Error()
	}
	if *modelName != "" {
		entry, resolveErr := a.catalog.Resolve(*modelName)
		if resolveErr != nil {
			report["model_error"] = resolveErr.Error()
		} else {
			report["model"] = entry.ID
			report["code_capable"] = integrations.ModelSupports(descriptor, entry)
			report["tool_calling_declared"] = hasCatalogCapability(entry, "tool-calling")
			_, installedErr := a.models.Installed(entry.ID)
			report["model_installed"] = installedErr == nil
			if resolved, metadataErr := a.models.ResolvePackage(ctx, entry); metadataErr == nil {
				report["context_tokens"] = resolved.Manifest.Model.ContextLength
				check, _ := integrations.CheckRecommendedContext(descriptor, resolved.Manifest.Model.ContextLength)
				report["context_status"] = check.Status
				report["context_reason"] = check.Reason
			} else {
				report["metadata_error"] = metadataErr.Error()
			}
		}
	}
	if state, stateErr := daemon.Read(a.paths); stateErr == nil {
		report["daemon_endpoint"] = state.Endpoint
		healthContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		healthErr := clientapi.New(state.Endpoint).Health(healthContext)
		cancel()
		report["daemon_running"] = healthErr == nil
		if healthErr != nil {
			report["daemon_error"] = healthErr.Error()
		}
	} else {
		report["daemon_running"] = false
	}
	report["config_isolated"] = true
	if descriptor.ID == "claude" {
		conflicts := make([]string, 0)
		for _, name := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CONFIG_DIR"} {
			if _, exists := os.LookupEnv(name); exists {
				conflicts = append(conflicts, name)
			}
		}
		report["routing_conflicts_overridden"] = conflicts
	}
	if *asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	}
	keys := []string{"integration", "installed", "version", "executable", "required_api", "model", "model_installed", "code_capable", "tool_calling_declared", "context_tokens", "context_status", "compute", "compute_configured", "daemon_running", "daemon_endpoint", "config_isolated", "routing_conflicts_overridden", "agent_qualified"}
	for _, key := range keys {
		if value, exists := report[key]; exists {
			fmt.Fprintf(a.out, "%-24s %v\n", key+":", value)
		}
	}
	if value, exists := report["install_instructions"]; exists {
		fmt.Fprintln(a.out, value)
	}
	return nil
}

func (a *app) targetStore() interface {
	Get(string) (compute.SSHConfig, error)
} {
	return compute.NewTargetStore(a.paths)
}

func selectCodingModel(input io.Reader, output io.Writer, models []catalog.Model, interactive bool) (string, error) {
	items := integrations.FilterCodingModels(models)
	if len(items) == 0 {
		return "", fmt.Errorf("the trusted catalog contains no models with an explicit code capability")
	}
	if !interactive {
		return "", fmt.Errorf("--model is required when stdin is not an interactive terminal")
	}
	fmt.Fprintln(output, "Choose a code-capable Backpack model (agent tool calling may still be unqualified):")
	for index, model := range items {
		fmt.Fprintf(output, "  %d. %-32s %s\n", index+1, model.ID, model.Status)
	}
	fmt.Fprint(output, "> ")
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return "", fmt.Errorf("read model selection: %w", scanner.Err())
	}
	choice := strings.TrimSpace(scanner.Text())
	if number, err := strconv.Atoi(choice); err == nil && number >= 1 && number <= len(items) {
		return items[number-1].ID, nil
	}
	for _, model := range items {
		if strings.EqualFold(choice, model.ID) {
			return model.ID, nil
		}
		for _, alias := range model.Aliases {
			if strings.EqualFold(choice, alias) {
				return model.ID, nil
			}
		}
	}
	return "", fmt.Errorf("unknown code-capable model selection %q", choice)
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func hasCatalogCapability(model catalog.Model, capability string) bool {
	for _, item := range model.Capabilities {
		if strings.EqualFold(item, capability) {
			return true
		}
	}
	return false
}

func integrationInstallInstructions(id string) string {
	switch id {
	case "claude":
		return "Install Claude Code from the official instructions: https://code.claude.com/docs/en/setup"
	case "codex":
		return "Install Codex CLI from the official package: npm install -g @openai/codex"
	case "opencode":
		return "Install OpenCode from the official instructions: https://opencode.ai/docs/"
	default:
		return "Install the integration tool from its official source and ensure it is on PATH."
	}
}
