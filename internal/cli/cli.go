package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/backpack-run/backpack-runtime/internal/adapters/llamacpp"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/server"
	"io"
	"net/http"
	"os"
	"os/signal"
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
	registry *backruntime.Registry
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
	llama := &llamacpp.Adapter{Paths: paths}
	a := &app{out, errOut, version, c, paths, manager, compute.Local{}, llama, nil}
	a.registry = backruntime.NewRegistry(llama)
	if len(args) == 0 {
		return a.help()
	}
	switch args[0] {
	case "help", "--help", "-h":
		return a.help()
	case "version":
		fmt.Fprintln(out, "backpack", version)
		return nil
	case "models":
		return a.catalogList()
	case "pull":
		return a.pull(ctx, args[1:])
	case "list":
		return a.list()
	case "inspect":
		return a.inspect(args[1:])
	case "hardware":
		return a.hardware(ctx)
	case "run":
		return a.run(ctx, args[1:])
	case "serve":
		return a.serve(ctx, args[1:])
	case "ps":
		fmt.Fprintln(out, "No sessions are owned by this CLI process. Persistent session state lands in the next milestone.")
		return nil
	case "stop":
		return fmt.Errorf("persistent session stop is not implemented; Ctrl+C stops foreground runs")
	default:
		return fmt.Errorf("unknown command %q; run `backpack help`", args[0])
	}
}
func (a *app) help() error {
	fmt.Fprint(a.out, `Backpack Runtime

Usage: backpack <command>

  models                  list the official catalog
  pull <model>            download and verify a package
  list                    list installed packages
  inspect <model>         show manifest/runtime compatibility
  hardware                inspect local compute
  run <model> [flags]     launch llama.cpp and optionally infer
  serve [--address addr]  start the loopback runtime API
  ps | stop               process commands (partial)
  version

Run flags: --prompt text --context tokens --port port --gpu-layers n
`)
	return nil
}
func (a *app) catalogList() error {
	fmt.Fprintf(a.out, "Official catalog %s\n\n", a.catalog.CatalogVersion)
	for _, m := range a.catalog.Models {
		alias := m.ID
		if len(m.Aliases) > 0 {
			alias = m.Aliases[0]
		}
		fmt.Fprintf(a.out, "%-26s %-16s %-24s %s\n", alias, m.RuntimeEngine, strings.Join(m.Capabilities, ","), m.Status)
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
func (a *app) inspect(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: backpack inspect <model>")
	}
	entry, err := a.catalog.Resolve(args[0])
	if err != nil {
		return err
	}
	m, err := a.models.Installed(entry.ID)
	if err != nil {
		return err
	}
	available := "available"
	if _, err = a.registry.Select(m.Runtime); err != nil {
		available = err.Error()
	}
	report := map[string]any{"id": m.ID, "repository": m.Repository, "revision": m.Revision, "package": m.Package, "runtime": m.Runtime, "adapter": available, "entrypoint": m.Entrypoint()}
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
	port := fs.Int("port", 0, "server port")
	gpu := fs.Int("gpu-layers", 99, "GPU layers")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack run <model> [--prompt text]")
	}
	entry, err := a.catalog.Resolve(modelName)
	if err != nil {
		return err
	}
	m, err := a.models.Installed(entry.ID)
	if err != nil {
		return err
	}
	adapter, err := a.registry.Select(m.Runtime)
	if err != nil {
		return err
	}
	if err = adapter.Prepare(ctx, m, a.local); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Backpack Runtime\n\nModel       %s\nRuntime     %s\nFormat      %s\nQuant       %s\nCompute     local\n\nLoading model...\n", m.Manifest.Model.DisplayName, m.Runtime.Engine, m.Package.Format, m.Package.Precision)
	session, err := adapter.Start(ctx, m, a.local, backruntime.StartOptions{Host: "127.0.0.1", Port: *port, ContextSize: *contextSize, GPULayers: *gpu})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Ready at %s\n", session.Endpoint)
	stop := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adapter.Stop(stopCtx, session)
	}
	defer stop()
	if *prompt != "" {
		answer, err := a.llama.Chat(ctx, session, *prompt)
		if err != nil {
			return err
		}
		fmt.Fprintln(a.out, answer)
		return nil
	}
	sigCtx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-sigCtx.Done()
	return nil
}
func (a *app) serve(ctx context.Context, args []string) error {
	if err := a.paths.Ensure(); err != nil {
		return err
	}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(a.err)
	address := fs.String("address", "127.0.0.1:11434", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s := &server.Server{Version: a.version, Catalog: a.catalog, Models: a.models, Hardware: func() (compute.Hardware, error) { return a.local.Inspect(ctx) }}
	fmt.Fprintln(a.out, "Backpack Runtime listening on http://"+*address)
	err := s.Listen(*address)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
