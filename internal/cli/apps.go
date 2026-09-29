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
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/cloud"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/daemon"
	"github.com/backpack-run/backpack-runtime/internal/integrations"
	clientapi "github.com/backpack-run/backpack-runtime/pkg/client"
)

func (a *app) runCommand(ctx context.Context, args []string) error {
	registry, err := integrations.Builtins()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack run <codex|codex-app|claude|claude-app|opencode|pi|list|doctor> [flags] [-- app-args]")
	}
	if args[0] == "list" {
		return a.appList(registry)
	}
	if args[0] == "doctor" {
		if len(args) < 2 {
			return fmt.Errorf("usage: backpack run doctor <claude|codex|opencode|pi> [--model model] [--compute target] [--json]")
		}
		return a.appDoctor(ctx, registry, args[1], args[2:])
	}
	if args[0] == "codex-app" {
		return a.runCodexApp(ctx, args[1:])
	}
	if args[0] == "claude-app" || args[0] == "claude-desktop" {
		return a.runClaudeApp(ctx, args[1:])
	}
	descriptor, err := registry.Get(args[0])
	if err != nil {
		return fmt.Errorf("unknown app %q; run `backpack run list`", args[0])
	}
	fs := flag.NewFlagSet("run "+descriptor.ID, flag.ContinueOnError)
	fs.SetOutput(a.err)
	modelName := fs.String("model", "", "Backpack coding model")
	computeName := fs.String("compute", "local", "Backpack compute target")
	contextTokens := fs.Int("context", 0, "context tokens; defaults to the model's largest execution-qualified window")
	keepAlive := fs.Bool("keep-alive", false, "leave the Backpack model session loaded after the external tool exits")
	force := fs.Bool("force", false, "run even when model fit recommends remote compute")
	if err = fs.Parse(args[1:]); err != nil {
		return err
	}
	if *modelName == "" {
		selected, selectErr := selectCodingModel(os.Stdin, a.out, a.catalog.Models, descriptor.ID, stdinIsTerminal())
		if selectErr != nil {
			return selectErr
		}
		*modelName = selected
	}
	if cloud.IsModel(*modelName) {
		return a.runCloudApp(ctx, descriptor, *modelName, *computeName, *contextTokens, *keepAlive, *force, fs.Args())
	}
	entry, err := a.catalog.Resolve(*modelName)
	if err != nil {
		return err
	}
	if !integrations.ModelSupports(descriptor, entry) {
		return fmt.Errorf("model %q is not eligible for %s: %s", entry.ID, descriptor.DisplayName, integrations.EligibilityReason(descriptor, entry))
	}
	if compatibility := integrations.Compatibility(descriptor, entry); compatibility.Status == "compatible-experimental" {
		fmt.Fprintf(a.err, "Warning: %s compatibility with %s is experimental and has not completed the full agent qualification suite.\n", entry.ID, descriptor.DisplayName)
	}
	resolved, err := a.models.ResolvePackage(ctx, entry)
	if err != nil {
		return err
	}
	modelContext := resolved.Manifest.Model.ContextLength
	desiredContext := *contextTokens
	desiredContext = integrations.ResolveContextWindow(desiredContext, modelContext, descriptor.RecommendedContextTokens)
	if modelContext > 0 && desiredContext > modelContext {
		return fmt.Errorf("requested context %d exceeds model maximum %d", desiredContext, modelContext)
	}
	contextCheck, _ := integrations.CheckRecommendedContext(descriptor, modelContext)
	if contextCheck.Status != integrations.ContextRecommended {
		fmt.Fprintf(a.err, "Warning: %s\n", contextCheck.Reason)
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
	if api.APIKey == "" {
		return fmt.Errorf("the running Backpack daemon predates authenticated agent routing; stop it and retry so this version can start a new daemon")
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
	options := integrations.ProviderOptions{Endpoint: api.BaseURL, Model: entry.ID, ContextTokens: desiredContext, ConfigDirectory: configDirectory, Executable: installation.Executable, Passthrough: fs.Args(), APIKey: api.APIKey}
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
	case "pi":
		invocation, err = integrations.PiInvocation(options)
	default:
		err = fmt.Errorf("app %q has no process runner", descriptor.ID)
	}
	if err != nil {
		return err
	}
	warnCodexWindowsSandboxFallback(a.err, descriptor.ID)
	fmt.Fprintf(a.out, "Starting %s through Backpack at %s (session %s).\n", descriptor.DisplayName, api.BaseURL, session.ID)
	return integrations.Run(ctx, invocation, integrations.ProcessIO{Stdin: os.Stdin, Stdout: a.out, Stderr: a.err})
}

func (a *app) runClaudeApp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run claude-app", flag.ContinueOnError)
	fs.SetOutput(a.err)
	modelName := fs.String("model", "", "Backpack coding model")
	computeName := fs.String("compute", "local", "Backpack compute target for local models")
	contextTokens := fs.Int("context", 0, "context tokens; defaults to the model's largest execution-qualified window")
	restore := fs.Bool("restore", false, "restore Claude App configuration from before Backpack setup")
	noOpen := fs.Bool("no-open", false, "configure or restore without opening Claude App")
	force := fs.Bool("force", false, "run a local model even when fit recommends remote compute")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("Claude App does not accept passthrough arguments")
	}
	stateDirectory, err := filepath.Abs(filepath.Join(a.paths.Config, "integrations", "claude-app"))
	if err != nil {
		return err
	}
	if *restore {
		if *modelName != "" || *contextTokens != 0 || *computeName != "local" || *force {
			return fmt.Errorf("--restore cannot be combined with model, context, compute, or force options")
		}
		if err = integrations.RestoreClaudeApp(stateDirectory); err != nil {
			return err
		}
		fmt.Fprintln(a.out, "Restored the Claude App configuration that was active before Backpack setup.")
		if *noOpen {
			return nil
		}
		return integrations.OpenClaudeApp()
	}
	if *modelName == "" {
		selected, selectErr := selectCodingModel(os.Stdin, a.out, a.catalog.Models, "claude", stdinIsTerminal())
		if selectErr != nil {
			return selectErr
		}
		*modelName = selected
	}

	desiredContext := *contextTokens
	cloudModel := cloud.IsModel(*modelName)
	var entry catalog.Model
	if cloudModel {
		if *computeName != "local" && *computeName != "cloud" {
			return fmt.Errorf("a :cloud model cannot use compute target %q", *computeName)
		}
		model, resolveErr := a.cloud.ResolveModel(ctx, *modelName)
		if resolveErr != nil {
			return resolveErr
		}
		if !model.HasCapability("code") || !model.HasCapability("tool-calling") {
			return fmt.Errorf("cloud model %q must declare code and tool-calling capabilities", model.ID)
		}
		desiredContext = integrations.ResolveContextWindow(desiredContext, model.ContextWindow, integrations.RecommendedAgentContext)
		if model.ContextWindow > 0 && desiredContext > model.ContextWindow {
			return fmt.Errorf("requested context %d exceeds cloud model maximum %d", desiredContext, model.ContextWindow)
		}
		entry = catalog.Model{ID: model.ID, DisplayName: model.DisplayName, Capabilities: append([]string(nil), model.Capabilities...), Status: model.Status}
	} else {
		entry, err = a.catalog.Resolve(*modelName)
		if err != nil {
			return err
		}
		descriptor, _ := integrations.Builtins()
		claude, _ := descriptor.Get("claude")
		if !integrations.ModelSupports(claude, entry) {
			return fmt.Errorf("model %q is not eligible for Claude App: %s", entry.ID, integrations.EligibilityReason(claude, entry))
		}
		resolved, resolveErr := a.models.ResolvePackage(ctx, entry)
		if resolveErr != nil {
			return resolveErr
		}
		maximum := resolved.Manifest.Model.ContextLength
		desiredContext = integrations.ResolveContextWindow(desiredContext, maximum, integrations.RecommendedAgentContext)
		if maximum > 0 && desiredContext > maximum {
			return fmt.Errorf("requested context %d exceeds model maximum %d", desiredContext, maximum)
		}
	}

	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	if api.APIKey == "" {
		return fmt.Errorf("the running Backpack daemon predates secure Claude App routing; stop it and retry")
	}
	if !cloudModel {
		fmt.Fprintf(a.out, "Loading %s on compute target %s...\n", entry.ID, *computeName)
		if _, err = api.CreateSession(ctx, clientapi.CreateSessionRequest{Model: entry.ID, Compute: *computeName, Options: clientapi.SessionOptions{ContextLength: desiredContext, GPULayers: "auto", Force: *force}}); err != nil {
			return err
		}
	}
	if err = integrations.ConfigureClaudeApp(integrations.ClaudeAppOptions{StateDirectory: stateDirectory, Endpoint: api.BaseURL, APIKey: api.APIKey, Model: entry.ID, ContextTokens: desiredContext}); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Configured Claude App to use %s through Backpack's authenticated loopback API.\n", entry.ID)
	fmt.Fprintln(a.out, "Restore with: backpack run claude-app --restore")
	if *noOpen {
		return nil
	}
	return integrations.OpenClaudeApp()
}

func (a *app) runCodexApp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run codex-app", flag.ContinueOnError)
	fs.SetOutput(a.err)
	modelName := fs.String("model", "", "Backpack coding model")
	computeName := fs.String("compute", "local", "Backpack compute target for local models")
	contextTokens := fs.Int("context", 0, "context tokens; defaults to the model's largest execution-qualified window")
	restore := fs.Bool("restore", false, "restore the exact Codex App config saved before Backpack setup")
	noOpen := fs.Bool("no-open", false, "configure or restore without opening Codex App")
	force := fs.Bool("force", false, "run a local model even when fit recommends remote compute")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("Codex App does not accept passthrough arguments")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return fmt.Errorf("Codex App is supported on Windows and macOS")
	}
	configPath, err := integrations.DefaultCodexAppConfigPath()
	if err != nil {
		return err
	}
	stateDirectory, err := filepath.Abs(filepath.Join(a.paths.Config, "integrations", "codex-app"))
	if err != nil {
		return err
	}
	if *restore {
		if *modelName != "" || *contextTokens != 0 || *computeName != "local" || *force {
			return fmt.Errorf("--restore cannot be combined with model, context, compute, or force options")
		}
		if err = integrations.RestoreCodexApp(configPath, stateDirectory); err != nil {
			return err
		}
		fmt.Fprintln(a.out, "Restored the Codex App configuration that was active before Backpack setup.")
		if *noOpen {
			return nil
		}
		fmt.Fprintln(a.out, "Opening Codex App. If it was already running, quit and reopen it to reload the restored configuration.")
		return integrations.OpenCodexApp()
	}
	if *modelName == "" {
		selected, selectErr := selectCodingModel(os.Stdin, a.out, a.catalog.Models, "codex", stdinIsTerminal())
		if selectErr != nil {
			return selectErr
		}
		*modelName = selected
	}

	var entry catalog.Model
	desiredContext := *contextTokens
	cloudModel := cloud.IsModel(*modelName)
	if cloudModel {
		if *computeName != "local" && *computeName != "cloud" {
			return fmt.Errorf("a :cloud model cannot use compute target %q", *computeName)
		}
		model, resolveErr := a.cloud.ResolveModel(ctx, *modelName)
		if resolveErr != nil {
			return resolveErr
		}
		if !model.HasCapability("code") || !model.HasCapability("tool-calling") {
			return fmt.Errorf("cloud model %q must declare code and tool-calling capabilities", model.ID)
		}
		desiredContext = integrations.ResolveContextWindow(desiredContext, model.ContextWindow, integrations.RecommendedAgentContext)
		if model.ContextWindow > 0 && desiredContext > model.ContextWindow {
			return fmt.Errorf("requested context %d exceeds cloud model maximum %d", desiredContext, model.ContextWindow)
		}
		entry = catalog.Model{ID: model.ID, DisplayName: model.DisplayName, Capabilities: append([]string(nil), model.Capabilities...), Status: model.Status}
	} else {
		entry, err = a.catalog.Resolve(*modelName)
		if err != nil {
			return err
		}
		descriptor, _ := integrations.Builtins()
		codex, _ := descriptor.Get("codex")
		if !integrations.ModelSupports(codex, entry) {
			return fmt.Errorf("model %q is not eligible for Codex App: %s", entry.ID, integrations.EligibilityReason(codex, entry))
		}
		resolved, resolveErr := a.models.ResolvePackage(ctx, entry)
		if resolveErr != nil {
			return resolveErr
		}
		modelContext := resolved.Manifest.Model.ContextLength
		desiredContext = integrations.ResolveContextWindow(desiredContext, modelContext, integrations.RecommendedAgentContext)
		if modelContext > 0 && desiredContext > modelContext {
			return fmt.Errorf("requested context %d exceeds model maximum %d", desiredContext, modelContext)
		}
	}

	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	if api.APIKey == "" {
		return fmt.Errorf("the running Backpack daemon predates secure Codex App routing; stop it and retry so this version can start a new daemon")
	}
	var createdSession string
	if !cloudModel {
		fmt.Fprintf(a.out, "Loading %s on compute target %s...\n", entry.ID, *computeName)
		session, createErr := api.CreateSession(ctx, clientapi.CreateSessionRequest{Model: entry.ID, Compute: *computeName, Options: clientapi.SessionOptions{ContextLength: desiredContext, GPULayers: "auto", Force: *force}})
		if createErr != nil {
			return createErr
		}
		createdSession = session.ID
	}
	configured := false
	if createdSession != "" {
		defer func() {
			if configured {
				return
			}
			stopContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = api.StopSession(stopContext, createdSession)
		}()
	}
	if err = integrations.ConfigureCodexApp(integrations.CodexAppOptions{ConfigPath: configPath, StateDirectory: stateDirectory, Endpoint: api.BaseURL, APIKey: api.APIKey, Model: entry, ContextTokens: desiredContext}); err != nil {
		return err
	}
	configured = true
	fmt.Fprintf(a.out, "Configured Codex App to use %s through Backpack's authenticated loopback API.\n", entry.ID)
	fmt.Fprintln(a.out, "Your Codex authentication file was not read or modified. Restore with: backpack run codex-app --restore")
	if *noOpen {
		return nil
	}
	fmt.Fprintln(a.out, "Opening Codex App. If it was already running, quit and reopen it so the new model catalog is loaded.")
	return integrations.OpenCodexApp()
}

func (a *app) runCloudApp(ctx context.Context, descriptor integrations.Descriptor, modelName, computeName string, contextTokens int, keepAlive, force bool, passthrough []string) error {
	if computeName != "local" && computeName != "cloud" {
		return fmt.Errorf("a :cloud model cannot use compute target %q", computeName)
	}
	if keepAlive {
		return fmt.Errorf("--keep-alive is not applicable to stateless Backpack Cloud inference")
	}
	if force {
		return fmt.Errorf("--force is not applicable to Backpack Cloud inference")
	}
	model, err := a.cloud.ResolveModel(ctx, modelName)
	if err != nil {
		return err
	}
	for _, capability := range descriptor.RequiredModelCapabilities {
		cloudCapability := capability
		if capability == "coding" {
			cloudCapability = "code"
		}
		if !model.HasCapability(cloudCapability) {
			return fmt.Errorf("cloud model %q does not declare the required %q capability", model.ID, cloudCapability)
		}
	}
	desiredContext := contextTokens
	desiredContext = integrations.ResolveContextWindow(desiredContext, model.ContextWindow, descriptor.RecommendedContextTokens)
	if model.ContextWindow > 0 && desiredContext > model.ContextWindow {
		return fmt.Errorf("requested context %d exceeds cloud model maximum %d", desiredContext, model.ContextWindow)
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
	if api.APIKey == "" {
		return fmt.Errorf("the running Backpack daemon predates secure Cloud proxying; stop it and retry so this version can start a new daemon")
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
	entry := catalog.Model{ID: model.ID, DisplayName: model.DisplayName, Capabilities: append([]string(nil), model.Capabilities...), Status: model.Status}
	options := integrations.ProviderOptions{Endpoint: api.BaseURL, Model: model.ID, ContextTokens: desiredContext, ConfigDirectory: configDirectory, Executable: installation.Executable, Passthrough: passthrough, APIKey: api.APIKey}
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
	case "pi":
		invocation, err = integrations.PiInvocation(options)
	default:
		err = fmt.Errorf("app %q has no process runner", descriptor.ID)
	}
	if err != nil {
		return err
	}
	warnCodexWindowsSandboxFallback(a.err, descriptor.ID)
	fmt.Fprintf(a.out, "Starting %s through Backpack Cloud model %s via the local loopback API.\n", descriptor.DisplayName, model.ID)
	return integrations.Run(ctx, invocation, integrations.ProcessIO{Stdin: os.Stdin, Stdout: a.out, Stderr: a.err})
}

func warnCodexWindowsSandboxFallback(output io.Writer, integrationID string) {
	if runtime.GOOS == "windows" && integrationID == "codex" {
		fmt.Fprintln(output, "Warning: using Codex's unelevated Windows sandbox fallback; filesystem restrictions remain, but isolation is weaker than the preferred elevated sandbox.")
	}
}

func (a *app) appList(registry *integrations.Registry) error {
	fmt.Fprintln(a.out, "APP          TOOL         PROTOCOL          INSTALLATION                    ELIGIBLE MODELS")
	discovery := integrations.NewDiscovery()
	for _, descriptor := range registry.List() {
		_, err := discovery.Detect(descriptor)
		status := "not installed"
		if err == nil {
			status = "installed"
		}
		eligible := 0
		for _, model := range a.catalog.Models {
			if integrations.ModelSupports(descriptor, model) {
				eligible++
			}
		}
		fmt.Fprintf(a.out, "%-12s %-12s %-17s %-31s %d\n", descriptor.ID, descriptor.DisplayName, descriptor.Protocol, status, eligible)
	}
	status := "supported on Windows/macOS"
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		status = "unsupported on this platform"
	}
	for _, app := range []struct{ id, name, agent, protocol string }{{"codex-app", "Codex App", "codex", "responses"}, {"claude-app", "Claude App", "claude", "messages"}} {
		descriptor, _ := registry.Get(app.agent)
		eligible := 0
		for _, model := range a.catalog.Models {
			if integrations.ModelSupports(descriptor, model) {
				eligible++
			}
		}
		fmt.Fprintf(a.out, "%-12s %-12s %-17s %-31s %d\n", app.id, app.name, app.protocol, status, eligible)
	}
	return nil
}

func (a *app) appDoctor(ctx context.Context, registry *integrations.Registry, integrationID string, args []string) error {
	descriptor, err := registry.Get(integrationID)
	if err != nil {
		return fmt.Errorf("unknown app %q; run `backpack run list`", integrationID)
	}
	fs := flag.NewFlagSet("run doctor", flag.ContinueOnError)
	fs.SetOutput(a.err)
	modelName := fs.String("model", "", "Backpack coding model")
	computeName := fs.String("compute", "local", "Backpack compute target")
	asJSON := fs.Bool("json", false, "print machine-readable diagnostics")
	if err = fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack run doctor %s [--model model] [--compute target] [--json]", integrationID)
	}
	report := map[string]any{"app": descriptor.ID, "display_name": descriptor.DisplayName, "required_api": descriptor.Protocol, "required_capabilities": descriptor.RequiredModelCapabilities, "compute": *computeName, "app_eligible": false, "app_qualified": false}
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
	cloudModel := cloud.IsModel(*modelName)
	if cloudModel && (*computeName == "local" || *computeName == "cloud") {
		report["compute"] = "cloud"
		report["compute_configured"] = true
	} else if *computeName == "local" {
		report["compute_configured"] = true
	} else if _, targetErr := a.targetStore().Get(*computeName); targetErr == nil {
		report["compute_configured"] = true
	} else {
		report["compute_configured"] = false
		report["compute_error"] = targetErr.Error()
	}
	if *modelName != "" {
		if cloudModel {
			cloudModelInfo, resolveErr := a.cloud.ResolveModel(ctx, *modelName)
			if resolveErr != nil {
				report["model_error"] = resolveErr.Error()
			} else {
				report["model"] = cloudModelInfo.ID
				report["model_status"] = cloudModelInfo.Status
				report["code_capable"] = cloudModelInfo.HasCapability("code")
				report["tool_calling_declared"] = cloudModelInfo.HasCapability("tool-calling")
				report["app_qualified"] = cloudModelInfo.Status == "available" && cloudModelInfo.HasCapability("code") && cloudModelInfo.HasCapability("tool-calling")
				report["app_eligible"] = report["app_qualified"]
				report["context_tokens"] = cloudModelInfo.ContextWindow
				report["model_installed"] = "not-applicable"
			}
		} else {
			entry, resolveErr := a.catalog.Resolve(*modelName)
			if resolveErr != nil {
				report["model_error"] = resolveErr.Error()
			} else {
				report["model"] = entry.ID
				report["code_capable"] = hasCatalogCapability(entry, "coding")
				report["tool_calling_declared"] = hasCatalogCapability(entry, "tool-calling")
				compatibility := integrations.Compatibility(descriptor, entry)
				report["compatibility_status"] = compatibility.Status
				report["compatibility_reason"] = compatibility.Reason
				report["app_eligible"] = integrations.ModelSupports(descriptor, entry)
				if reason := integrations.EligibilityReason(descriptor, entry); reason != "" {
					report["eligibility_error"] = reason
				}
				report["app_qualified"] = compatibility.Status == "qualified"
				installed, installedErr := a.models.Installed(entry.ID)
				report["model_installed"] = installedErr == nil
				resolved := installed
				var metadataErr error
				if resolved == nil {
					resolved, metadataErr = a.models.ResolvePackage(ctx, entry)
				}
				if metadataErr == nil {
					contextTokens := resolved.Manifest.Model.ContextLength
					report["context_tokens"] = contextTokens
					check, _ := integrations.CheckRecommendedContext(descriptor, contextTokens)
					report["context_status"] = check.Status
					report["context_reason"] = check.Reason
				} else {
					report["metadata_error"] = metadataErr.Error()
				}
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
	keys := []string{"app", "installed", "version", "executable", "required_api", "model", "model_installed", "code_capable", "tool_calling_declared", "compatibility_status", "eligibility_error", "context_tokens", "context_status", "compute", "compute_configured", "daemon_running", "daemon_endpoint", "config_isolated", "routing_conflicts_overridden", "app_eligible", "app_qualified"}
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

func selectCodingModel(input io.Reader, output io.Writer, models []catalog.Model, agentID string, interactive bool) (string, error) {
	registry, _ := integrations.Builtins()
	descriptor, err := registry.Get(agentID)
	if err != nil {
		return "", err
	}
	items := make([]catalog.Model, 0)
	for _, model := range models {
		if integrations.ModelSupports(descriptor, model) {
			items = append(items, model)
		}
	}
	if len(items) == 0 {
		return "", fmt.Errorf("the trusted catalog contains no models eligible for %s", descriptor.DisplayName)
	}
	if !interactive {
		return "", fmt.Errorf("--model is required when stdin is not an interactive terminal")
	}
	fmt.Fprintf(output, "Choose a Backpack model eligible for %s:\n", descriptor.DisplayName)
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
	case "pi":
		return "Install Pi from the official package: npm install -g --ignore-scripts @earendil-works/pi-coding-agent"
	default:
		return "Install the integration tool from its official source and ensure it is on PATH."
	}
}
