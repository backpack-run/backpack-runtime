package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/backpack-run/backpack-runtime/internal/adapters/llamacpp"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/catalogverify"
	"github.com/backpack-run/backpack-runtime/internal/cloud"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/daemon"
	"github.com/backpack-run/backpack-runtime/internal/diagnostics"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/fit"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/runtimebundle"
	"github.com/backpack-run/backpack-runtime/internal/server"
	"github.com/backpack-run/backpack-runtime/internal/sessions"
	clientapi "github.com/backpack-run/backpack-runtime/pkg/client"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
)

type app struct {
	out, err io.Writer
	version  string
	catalog  catalog.Catalog
	paths    config.Paths
	models   *models.Manager
	local    compute.Local
	llama    *llamacpp.Adapter
	runtimes *runtimebundle.Manager
	registry *backruntime.Registry
	cloud    *cloud.Client
}

func Run(ctx context.Context, args []string, out, errOut io.Writer, version string) error {
	c, err := catalog.Load()
	if err != nil {
		return err
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	manager := models.NewManager(paths)
	runtimes, err := runtimebundle.New(paths, nil)
	if err != nil {
		return err
	}
	cloudClient, err := cloud.New(paths)
	if err != nil {
		cloudClient = cloud.Unavailable(paths, err)
	}
	llama := &llamacpp.Adapter{Paths: paths, Runtimes: runtimes}
	a := &app{out: out, err: errOut, version: version, catalog: c, paths: paths, models: manager, local: compute.Local{}, llama: llama, runtimes: runtimes, cloud: cloudClient}
	a.registry = backruntime.NewRegistry(llama)
	if len(args) == 0 {
		return a.landing(ctx)
	}
	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h") {
		return a.commandHelp(args[0])
	}
	switch args[0] {
	case "help", "--help", "-h":
		return a.help()
	case "version", "--version", "-v":
		fmt.Fprintln(out, "backpack", version)
		return nil
	case "models":
		return a.catalogList(args[1:])
	case "model":
		return a.modelCommand(ctx, args[1:])
	case "catalog":
		return a.catalogCommand(ctx, args[1:])
	case "pull":
		return a.pull(ctx, args[1:])
	case "list":
		return a.list()
	case "inspect":
		return a.inspect(ctx, args[1:])
	case "hardware":
		return a.hardware(ctx)
	case "run":
		return a.run(ctx, args[1:])
	case "launch":
		return a.launchCommand(ctx, args[1:])
	case "login":
		return a.login(ctx, args[1:])
	case "logout":
		return a.logout(args[1:])
	case "cloud":
		return a.cloudCommand(ctx, args[1:])
	case "serve":
		return a.serve(ctx, args[1:], false)
	case "ps":
		return a.ps(ctx)
	case "stop":
		return a.stop(ctx, args[1:])
	case "compute":
		return a.compute(ctx, args[1:])
	case "runtime":
		return a.runtimeCommand(ctx, args[1:])
	case "doctor":
		return a.doctor(ctx, args[1:])
	case "update":
		return a.update(ctx, args[1:])
	case "_daemon":
		return a.serve(ctx, args[1:], true)
	default:
		return fmt.Errorf("unknown command %q; run `backpack help`", args[0])
	}
}

func (a *app) commandHelp(command string) error {
	usage := map[string]string{
		"models":   "Usage: backpack models [--json] [--all]\n\nList the curated coding-model catalog. --all also includes internal test fixtures.\n",
		"model":    "Usage: backpack model show <model>\n\nShow trusted package, runtime, and local-fit details.\n",
		"catalog":  "Usage: backpack catalog verify [--json]\n\nCompare the trusted catalog with public Backpack packages on Hugging Face.\n",
		"pull":     "Usage: backpack pull <model>\n\nResolve, download, verify, and atomically install a model package.\n",
		"list":     "Usage: backpack list\n\nList installed model packages.\n",
		"inspect":  "Usage: backpack inspect <model>\n\nInspect package metadata, runtime compatibility, and local fit without downloading weights.\n",
		"hardware": "Usage: backpack hardware\n\nInspect local CPU, memory, GPU, and runtime capabilities.\n",
		"run":      "Usage: backpack run <model> [--prompt text] [--context tokens] [--gpu-layers auto|n] [--keep-alive] [--detach] [--force]\n",
		"launch":   "Usage: backpack launch <list|doctor|claude|claude-app|codex|codex-app|opencode|pi> [options] [-- tool-args]\n\nLaunch a supported coding agent through an eligible open coding model. App setup is persistent; restore it with `backpack launch <codex-app|claude-app> --restore`.\n",
		"login":    "Usage: backpack login [--name device-name] [--no-browser]\n\nOptionally authorize this device for Backpack Cloud. Open-source Backpack requires no account.\n",
		"logout":   "Usage: backpack logout\n\nRemove the local Backpack Cloud device credential.\n",
		"cloud":    "Usage: backpack cloud <status|models> [--json]\n\nInspect Backpack Cloud authentication and live model availability.\n",
		"serve":    "Usage: backpack serve [--address 127.0.0.1:port]\n\nRun the local HTTP service in the foreground.\n",
		"ps":       "Usage: backpack ps\n\nList service-owned runtime sessions.\n",
		"stop":     "Usage: backpack stop <session-id>\n\nGracefully stop one exact session.\n",
		"compute":  "Usage: backpack compute <list|add|show|test|doctor|remove> [arguments]\n",
		"runtime":  "Usage: backpack runtime <list|show|install|verify|remove> [arguments]\n",
		"doctor":   "Usage: backpack doctor [--json]\n\nPrint sanitized local diagnostics suitable for bug reports.\n",
		"update":   "Usage: backpack update [--check] [--version version | --prerelease]\n\nExplicitly check for or install a SHA-256-verified GitHub release. Stable releases are selected by default.\n",
		"version":  "Usage: backpack version\n",
	}
	text, ok := usage[command]
	if !ok {
		return a.help()
	}
	fmt.Fprint(a.out, text)
	return nil
}

func (a *app) help() error {
	fmt.Fprint(a.out, `Backpack Runtime

Usage: backpack <command>

  models [--json] [--all] list the curated coding catalog and local status
  model show <model>      show model package, runtime, and fit details
  catalog verify          detect trusted catalog/Hugging Face drift
  pull <model>            download and verify a package
  list                    list installed packages
  inspect <model>         show manifest/runtime compatibility
  hardware                inspect local compute
  run <model> [flags]     create an API-owned session and chat
  launch <integration>    launch a coding agent through an eligible open model
  login                   optionally authorize this device for Backpack Cloud
  logout                  remove the local Backpack Cloud credential
  cloud <command>         inspect Cloud authentication and live models
  serve [--address addr]  start the loopback runtime API
  ps                      list runtime-owned sessions
  stop <session>          gracefully stop a session
  compute <command>       manage local and SSH compute targets
  runtime <command>       inspect and manage inference runtimes
  doctor [--json]         print sanitized release diagnostics
  update [flags]          explicitly check for or install a verified release
  version

Run flags: --prompt text --context tokens --gpu-layers auto|n --keep-alive --detach --force
`)
	return nil
}

func (a *app) watchEvents(ctx context.Context, api *clientapi.Client) context.CancelFunc {
	eventContext, cancel := context.WithCancel(ctx)
	go func() {
		_ = api.Events(eventContext, func(event clientapi.Event) {
			if event.Kind == string(events.Progress) && event.Total > 0 {
				fmt.Fprintf(a.err, "\r%s %6.1f%%", event.Message, event.Percentage)
				return
			}
			if event.Message != "" {
				fmt.Fprintln(a.err, event.Message)
			}
		})
	}()
	return cancel
}

func (a *app) runtimeCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack runtime <list|show|install|verify|remove>")
	}
	items, err := a.runtimes.List()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		if len(items) == 0 {
			fmt.Fprintln(a.out, "No managed runtimes installed.")
			return nil
		}
		for _, x := range items {
			fmt.Fprintf(a.out, "%-14s %-10s %-24s %-8s %s\n", x.Engine, x.Version, x.Variant, x.Backend, x.Directory)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack runtime show <engine>")
		}
		h, err := a.local.Inspect(ctx)
		if err != nil {
			return err
		}
		r, v, err := a.runtimes.Resolve(models.RuntimeRequirement{Engine: args[1], Environment: "native-bundle"}, h)
		if err != nil {
			return err
		}
		report := map[string]any{"runtime": r, "selected_variant": v}
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(a.out, string(b))
		return nil
	case "install":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack runtime install <engine>")
		}
		a.runtimes.Sink = func(e events.Event) {
			if e.Kind == events.Progress {
				fmt.Fprintf(a.out, "\rDownloading %-35s %d bytes", e.Message, e.Current)
			} else {
				fmt.Fprintln(a.out, e.Message)
			}
		}
		x, err := a.runtimes.Ensure(ctx, models.RuntimeRequirement{Engine: args[1], Environment: "native-bundle"}, a.local)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "\nReady: %s %s (%s)\n", x.Engine, x.Version, x.Variant)
		return nil
	case "verify":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack runtime verify <engine>")
		}
		matched := false
		for _, x := range items {
			if strings.EqualFold(x.Engine, args[1]) {
				matched = true
				if err := a.runtimes.Verify(x); err != nil {
					return err
				}
				fmt.Fprintf(a.out, "Verified %s %s (%s)\n", x.Engine, x.Version, x.Variant)
			}
		}
		if !matched {
			return fmt.Errorf("runtime %q is not installed", args[1])
		}
		return nil
	case "remove":
		if len(args) != 4 {
			return fmt.Errorf("usage: backpack runtime remove <engine> <version> <variant>")
		}
		return a.runtimes.Remove(args[1], args[2], args[3])
	default:
		return fmt.Errorf("unknown runtime command %q", args[0])
	}
}
func (a *app) doctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(a.err)
	asJSON := fs.Bool("json", false, "emit sanitized machine-readable diagnostics")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack doctor [--json]")
	}
	report := diagnostics.Collect(ctx, a.version, a.paths, a.local, a.models, a.runtimes, compute.NewTargetStore(a.paths), a.cloud)
	report.Update = a.updateDiagnostic(ctx)
	if *asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	}
	fmt.Fprintf(a.out, "Backpack %s\nPlatform: %s\nBinary: %s (on PATH: %v)\nUpdate: %s", report.Version, report.Platform, report.Binary.Path, report.Binary.InPath, report.Update.Status)
	if report.Update.Target != "" {
		fmt.Fprintf(a.out, " (%s)", report.Update.Target)
	}
	fmt.Fprintf(a.out, "\nHome: %s\nDaemon: %v\nModels: %d installed, %d verified, %d corrupt\nRuntimes: %d installed, %d verified, %d corrupt\nCompute targets: %d\n", report.BackpackHome, report.Daemon.Running, report.Models.Installed, report.Models.Verified, report.Models.Corrupt, report.Runtimes.Installed, report.Runtimes.Verified, report.Runtimes.Corrupt, len(report.ComputeTargets))
	fmt.Fprintf(a.out, "Cloud: %v (%s)\n", report.Cloud.Authenticated, report.Cloud.CredentialSource)
	for _, integration := range report.Integrations {
		status := "not found"
		if integration.Installed {
			status = integration.Version
			if status == "" {
				status = "installed"
			}
		}
		fmt.Fprintf(a.out, "Agent %s: %s\n", integration.ID, status)
	}
	if len(report.ConfigurationOverrides) > 0 {
		fmt.Fprintf(a.out, "Configuration overrides: %s\n", strings.Join(report.ConfigurationOverrides, ", "))
	}
	for _, problem := range report.KnownProblems {
		fmt.Fprintln(a.out, "Problem:", problem)
	}
	return nil
}

type catalogModelSummary struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Capabilities []string `json:"capabilities"`
	Runtime      string   `json:"runtime"`
	Status       string   `json:"status"`
	Installed    bool     `json:"installed"`
	SizeBytes    int64    `json:"size_bytes,omitempty"`
	LocalFit     string   `json:"local_fit,omitempty"`
	Protocols    []string `json:"protocols,omitempty"`
	Context      int      `json:"context_window,omitempty"`
	Agents       []string `json:"compatible_agents,omitempty"`
}

func (a *app) catalogList(args []string) error {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(a.err)
	asJSON := fs.Bool("json", false, "emit machine-readable model summaries")
	showAll := fs.Bool("all", false, "include internal runtime test fixtures")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack models [--json] [--all]")
	}
	hardware, _ := a.local.Inspect(context.Background())
	summaries := make([]catalogModelSummary, 0, len(a.catalog.Models))
	for _, entry := range a.catalog.Models {
		if entry.Visibility == "test-fixture" && !*showAll {
			continue
		}
		name := entry.ID
		if len(entry.Aliases) > 0 {
			name = entry.Aliases[0]
		}
		agents := make([]string, 0, len(entry.Agents))
		for agent, compatibility := range entry.Agents {
			if compatibility.Status == "qualified" || compatibility.Status == "compatible-experimental" {
				agents = append(agents, agent)
			}
		}
		sort.Strings(agents)
		summary := catalogModelSummary{Name: name, DisplayName: entry.DisplayName, Capabilities: entry.Capabilities, Runtime: entry.RuntimeEngine, Status: entry.Status, Protocols: entry.Protocols, Context: entry.ContextWindow, Agents: agents}
		if installed, err := a.models.Installed(entry.ID); err == nil {
			summary.Installed = true
			summary.SizeBytes = installed.Package.SizeBytes
			if hardware.OS != "" {
				summary.LocalFit = string(fit.Evaluate(installed.Package, hardware).State)
			}
		}
		summaries = append(summaries, summary)
	}
	if *asJSON {
		encoded, _ := json.MarshalIndent(map[string]any{"catalog_version": a.catalog.CatalogVersion, "models": summaries}, "", "  ")
		fmt.Fprintln(a.out, string(encoded))
		return nil
	}
	fmt.Fprintf(a.out, "Official catalog %s\n\n", a.catalog.CatalogVersion)
	fmt.Fprintln(a.out, "MODEL                      CAPABILITIES                                  SIZE       FIT                 INSTALLED  STATUS")
	for _, summary := range summaries {
		size, localFit, installed := "-", "inspect", "no"
		if summary.Installed {
			size, localFit, installed = humanBytes(summary.SizeBytes), summary.LocalFit, "yes"
		}
		fmt.Fprintf(a.out, "%-26s %-45s %-10s %-19s %-10s %s\n", summary.Name, strings.Join(summary.Capabilities, ","), size, localFit, installed, summary.Status)
	}
	return nil
}

func (a *app) modelCommand(ctx context.Context, args []string) error {
	if len(args) == 2 && args[0] == "show" {
		return a.inspect(ctx, args[1:])
	}
	return fmt.Errorf("usage: backpack model show <model>")
}

func humanBytes(size int64) string {
	if size <= 0 {
		return "-"
	}
	const gib = 1024 * 1024 * 1024
	if size >= gib {
		return fmt.Sprintf("%.1f GiB", float64(size)/gib)
	}
	return fmt.Sprintf("%.0f MiB", float64(size)/(1024*1024))
}

func (a *app) catalogCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return fmt.Errorf("usage: backpack catalog verify [--json]")
	}
	fs := flag.NewFlagSet("catalog verify", flag.ContinueOnError)
	fs.SetOutput(a.err)
	asJSON := fs.Bool("json", false, "print machine-readable verification report")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack catalog verify [--json]")
	}
	report, err := (catalogverify.Verifier{}).Verify(ctx, a.catalog)
	if err != nil {
		return err
	}
	if *asJSON {
		encoded, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(a.out, string(encoded))
	} else {
		fmt.Fprintf(a.out, "%d HF packages discovered\n%d curated coding packages represented\n%d out-of-scope packages ignored\n", report.Discovered, report.Represented, report.Uncurated)
		statuses := make([]string, 0, len(report.StatusCounts))
		for status := range report.StatusCounts {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			fmt.Fprintf(a.out, "%d %s\n", report.StatusCounts[status], status)
		}
		for _, issue := range report.Issues {
			fmt.Fprintf(a.out, "- %s [%s]: %s\n", issue.Repository, issue.Code, issue.Message)
		}
	}
	if len(report.Issues) > 0 {
		return fmt.Errorf("catalog verification found %d issue(s)", len(report.Issues))
	}
	return nil
}
func (a *app) pull(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: backpack pull <model>")
	}
	m, err := a.catalog.Resolve(args[0])
	if err != nil {
		return err
	}
	installed, err := a.models.Pull(ctx, m, func(e events.Event) {
		if e.Kind == events.Progress {
			fmt.Fprintf(a.out, "\r%-42s %6.1f%%", e.Message, e.Percentage)
		} else {
			fmt.Fprintln(a.out, e.Message)
		}
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "\nInstalled %s (%s)\n", installed.ID, installed.Package.Precision)
	return nil
}
func (a *app) list() error {
	items, err := a.models.List()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(a.out, "No models installed.")
		return nil
	}
	for _, m := range items {
		fmt.Fprintf(a.out, "%-28s %-10s %-16s %s\n", m.ID, m.Package.Precision, m.Runtime.Engine, m.Directory)
	}
	return nil
}
func (a *app) inspect(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: backpack inspect <model>")
	}
	entry, err := a.catalog.Resolve(args[0])
	if err != nil {
		return err
	}
	m, err := a.models.Installed(entry.ID)
	installed := err == nil
	if !installed {
		m, err = a.models.ResolvePackage(ctx, entry)
		if err != nil {
			return err
		}
	}
	available := "available"
	if _, err = a.registry.Select(m.Runtime); err != nil {
		available = err.Error()
	}
	hardware, hardwareErr := a.local.Inspect(context.Background())
	var localFit any = "hardware inspection unavailable"
	if hardwareErr == nil {
		localFit = fit.Evaluate(m.Package, hardware)
	}
	report := map[string]any{"id": m.ID, "repository": m.Repository, "revision": m.Revision, "installed": installed, "package": m.Package, "runtime": m.Runtime, "adapter": available, "entrypoint": m.Package.RuntimeEntrypoint(), "local_fit": localFit}
	b, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintln(a.out, string(b))
	return nil
}
func (a *app) hardware(ctx context.Context) error {
	h, err := a.local.Inspect(ctx)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(h, "", "  ")
	fmt.Fprintln(a.out, string(b))
	return nil
}
func (a *app) run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack run <model> [--prompt text]")
	}
	modelName := args[0]
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.err)
	prompt := fs.String("prompt", "", "one-shot chat prompt")
	contextSize := fs.Int("context", 0, "context tokens")
	gpu := fs.String("gpu-layers", "auto", "GPU layers")
	computeName := fs.String("compute", "local", "compute target")
	keepAlive := fs.Bool("keep-alive", false, "leave the session loaded on exit")
	detach := fs.Bool("detach", false, "create the session and return")
	force := fs.Bool("force", false, "run even when model fit recommends remote compute")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack run <model> [--prompt text]")
	}
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, "Loading model...")
	stopEvents := a.watchEvents(ctx, api)
	defer stopEvents()
	session, err := api.CreateSession(ctx, clientapi.CreateSessionRequest{Model: modelName, Compute: *computeName, Options: clientapi.SessionOptions{ContextLength: *contextSize, GPULayers: *gpu, Force: *force}})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Ready. Session %s\n", session.ID)
	stop := func() {
		if *keepAlive || *detach {
			return
		}
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = api.StopSession(stopCtx, session.ID)
	}
	defer stop()
	if *detach {
		return nil
	}
	if *prompt != "" {
		err = api.Chat(ctx, modelName, *prompt, true, func(token string) { fmt.Fprint(a.out, token) })
		fmt.Fprintln(a.out)
		return err
	}
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(a.out, ">>> ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		prompt := strings.TrimSpace(scanner.Text())
		if prompt == "" {
			continue
		}
		if prompt == "/exit" || prompt == "/quit" {
			return nil
		}
		if err = api.Chat(ctx, modelName, prompt, true, func(token string) { fmt.Fprint(a.out, token) }); err != nil {
			return err
		}
		fmt.Fprintln(a.out)
	}
}

func (a *app) ps(ctx context.Context) error {
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	items, err := api.Sessions(ctx)
	if err != nil {
		return err
	}
	active := items[:0]
	for _, s := range items {
		if s.Status == "starting" || s.Status == "ready" || s.Status == "stopping" {
			active = append(active, s)
		}
	}
	items = active
	if len(items) == 0 {
		fmt.Fprintln(a.out, "No active sessions.")
		return nil
	}
	fmt.Fprintln(a.out, "SESSION                              MODEL                       RUNTIME      COMPUTE  STATUS    AGE")
	for _, s := range items {
		fmt.Fprintf(a.out, "%-36s %-27s %-12s %-8s %-9s %s\n", s.ID, s.ModelID, s.Runtime, s.Compute, s.Status, shortAge(time.Since(s.CreatedAt)))
	}
	return nil
}
func (a *app) stop(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: backpack stop <session>")
	}
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	stopCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	if err = api.StopSession(stopCtx, args[0]); err != nil {
		return err
	}
	fmt.Fprintln(a.out, "Stopped", args[0])
	return nil
}
func shortAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func (a *app) compute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack compute <list|show|add|test|remove>")
	}
	store := compute.NewTargetStore(a.paths)
	switch args[0] {
	case "list":
		items, err := store.List()
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, "NAME                 KIND  HOST")
		fmt.Fprintln(a.out, "local                local this computer")
		for _, item := range items {
			fmt.Fprintf(a.out, "%-20s ssh   %s\n", item.ID, item.Host)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack compute show <name>")
		}
		if args[1] == "local" {
			return a.hardware(ctx)
		}
		item, err := store.Get(args[1])
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(item, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	case "test", "doctor":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack compute test <name>")
		}
		if args[1] == "local" {
			return a.hardware(ctx)
		}
		item, err := store.Get(args[1])
		if err != nil {
			return err
		}
		h, err := compute.NewSSH(item).Doctor(ctx)
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(h, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack compute remove <name>")
		}
		if args[1] == "local" {
			return fmt.Errorf("the built-in local target cannot be removed")
		}
		if err := store.Remove(args[1]); err != nil {
			return err
		}
		fmt.Fprintln(a.out, "Removed", args[1])
		return nil
	case "add":
		return a.computeAdd(store, args[1:])
	default:
		return fmt.Errorf("unknown compute command %q", args[0])
	}
}

func (a *app) computeAdd(store compute.TargetStore, args []string) error {
	if len(args) < 2 || args[0] != "ssh" {
		return fmt.Errorf("usage: backpack compute add ssh <name> --host <host> [flags]")
	}
	name := args[1]
	fs := flag.NewFlagSet("compute add ssh", flag.ContinueOnError)
	fs.SetOutput(a.err)
	host := fs.String("host", "", "SSH host or config alias")
	user := fs.String("user", "", "SSH username")
	port := fs.Int("port", 22, "SSH port")
	identity := fs.String("identity", "", "identity file reference")
	root := fs.String("remote-root", ".backpack", "remote state directory under the user's home")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	if *host == "" {
		return fmt.Errorf("--host is required")
	}
	target := compute.SSHConfig{ID: name, Host: *host, User: *user, Port: *port, IdentityFile: *identity, RemoteRoot: *root}
	if err := store.Put(target); err != nil {
		return err
	}
	fmt.Fprintln(a.out, "Saved SSH compute target", name)
	return nil
}

func (a *app) serve(ctx context.Context, args []string, internal bool) error {
	if err := a.paths.Ensure(); err != nil {
		return err
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(a.err)
	address := fs.String("address", "127.0.0.1:11434", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apiKey := ""
	if internal {
		apiKey = strings.TrimSpace(os.Getenv("BACKPACK_DAEMON_API_KEY"))
		if !daemon.ValidAPIKey(apiKey) {
			return fmt.Errorf("internal daemon API key is missing or invalid")
		}
	} else {
		var err error
		apiKey, err = daemon.NewAPIKey()
		if err != nil {
			return err
		}
	}
	broker := events.NewBroker()
	a.runtimes.Sink = broker.Publish
	manager := sessions.NewWithEvents(a.catalog, a.models, a.registry, a.paths, broker.Publish)
	s := &server.Server{Version: a.version, Catalog: a.catalog, Models: a.models, Sessions: manager, Events: broker, Targets: compute.NewTargetStore(a.paths), Cloud: a.cloud, CloudProxyToken: apiKey, Hardware: func() (compute.Hardware, error) { return a.local.Inspect(ctx) }}
	if existing, err := daemon.Read(a.paths); err == nil && existing.PID != os.Getpid() {
		check, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		healthErr := clientapi.New(existing.Endpoint).Health(check)
		cancel()
		if healthErr == nil {
			return fmt.Errorf("Backpack Runtime is already running at %s", existing.Endpoint)
		}
	}
	if err := daemon.Write(a.paths, daemon.State{PID: os.Getpid(), Endpoint: "http://" + *address, StartedAt: time.Now().UTC(), Version: a.version, APIKey: apiKey}); err != nil {
		return err
	}
	defer daemon.ClearIfOwned(a.paths, os.Getpid())
	fmt.Fprintln(a.out, "Backpack Runtime listening on http://"+*address)
	sigCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err := s.Serve(sigCtx, *address)
	shutdownCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
	defer stop()
	manager.Shutdown(shutdownCtx)
	return err
}
