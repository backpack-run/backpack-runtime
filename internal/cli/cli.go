package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/backpack-run/backpack-runtime/internal/adapters/llamacpp"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/daemon"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/models"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/server"
	"github.com/backpack-run/backpack-runtime/internal/sessions"
	clientapi "github.com/backpack-run/backpack-runtime/pkg/client"
	"io"
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
		return a.ps(ctx)
	case "stop":
		return a.stop(ctx, args[1:])
	case "compute":
		return a.compute(ctx, args[1:])
	case "_daemon":
		return a.serve(ctx, args[1:])
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
  run <model> [flags]     create an API-owned session and chat
  serve [--address addr]  start the loopback runtime API
  ps                      list runtime-owned sessions
  stop <session>          gracefully stop a session
  compute <command>       manage local and SSH compute targets
  version

Run flags: --prompt text --context tokens --gpu-layers auto|n --keep-alive --detach
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
	gpu := fs.String("gpu-layers", "auto", "GPU layers")
	computeName := fs.String("compute", "local", "compute target")
	keepAlive := fs.Bool("keep-alive", false, "leave the session loaded on exit")
	detach := fs.Bool("detach", false, "create the session and return")
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
	session, err := api.CreateSession(ctx, clientapi.CreateSessionRequest{Model: modelName, Compute: *computeName, Options: clientapi.SessionOptions{ContextLength: *contextSize, GPULayers: *gpu}})
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
	case "test":
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
		h, err := compute.NewSSH(item).Inspect(ctx)
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
	manager := sessions.New(a.catalog, a.models, a.registry, a.paths)
	s := &server.Server{Version: a.version, Catalog: a.catalog, Models: a.models, Sessions: manager, Targets: compute.NewTargetStore(a.paths), Hardware: func() (compute.Hardware, error) { return a.local.Inspect(ctx) }}
	if existing, err := daemon.Read(a.paths); err == nil && existing.PID != os.Getpid() {
		check, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		healthErr := clientapi.New(existing.Endpoint).Health(check)
		cancel()
		if healthErr == nil {
			return fmt.Errorf("Backpack Runtime is already running at %s", existing.Endpoint)
		}
	}
	if err := daemon.Write(a.paths, daemon.State{PID: os.Getpid(), Endpoint: "http://" + *address, StartedAt: time.Now().UTC(), Version: a.version}); err != nil {
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
