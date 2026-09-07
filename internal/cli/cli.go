package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/backpack-run/backpack-runtime/internal/adapters/llamacpp"
	"github.com/backpack-run/backpack-runtime/internal/adapters/pythonworker"
	"github.com/backpack-run/backpack-runtime/internal/adapters/whispercpp"
	"github.com/backpack-run/backpack-runtime/internal/catalog"
	"github.com/backpack-run/backpack-runtime/internal/catalogverify"
	"github.com/backpack-run/backpack-runtime/internal/compute"
	"github.com/backpack-run/backpack-runtime/internal/config"
	"github.com/backpack-run/backpack-runtime/internal/daemon"
	"github.com/backpack-run/backpack-runtime/internal/diagnostics"
	"github.com/backpack-run/backpack-runtime/internal/events"
	"github.com/backpack-run/backpack-runtime/internal/fit"
	"github.com/backpack-run/backpack-runtime/internal/jobs"
	"github.com/backpack-run/backpack-runtime/internal/models"
	"github.com/backpack-run/backpack-runtime/internal/pythonruntime"
	backruntime "github.com/backpack-run/backpack-runtime/internal/runtime"
	"github.com/backpack-run/backpack-runtime/internal/runtimebundle"
	"github.com/backpack-run/backpack-runtime/internal/server"
	"github.com/backpack-run/backpack-runtime/internal/sessions"
	clientapi "github.com/backpack-run/backpack-runtime/pkg/client"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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
	whisper  *whispercpp.Adapter
	qwenASR  *pythonworker.Adapter
	kokoro   *pythonworker.Adapter
	runtimes *runtimebundle.Manager
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
	runtimes, err := runtimebundle.New(paths, nil)
	if err != nil {
		return err
	}
	llama := &llamacpp.Adapter{Paths: paths, Runtimes: runtimes}
	whisper := &whispercpp.Adapter{Paths: paths, Runtimes: runtimes}
	pythonEnvironments := &pythonruntime.Manager{Paths: paths, Runtimes: runtimes}
	qwenASR := &pythonworker.Adapter{Engine: "qwen-asr", Provides: []string{"transcription"}, Paths: paths, Environments: pythonEnvironments}
	kokoro := &pythonworker.Adapter{Engine: "kokoro", Provides: []string{"speech"}, Paths: paths, Environments: pythonEnvironments}
	a := &app{out: out, err: errOut, version: version, catalog: c, paths: paths, models: manager, local: compute.Local{}, llama: llama, whisper: whisper, qwenASR: qwenASR, kokoro: kokoro, runtimes: runtimes}
	a.registry = backruntime.NewRegistry(llama, whisper, qwenASR, kokoro)
	if len(args) == 0 {
		return a.help()
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
		return a.catalogList()
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
	case "transcribe":
		return a.transcribe(ctx, args[1:])
	case "speak":
		return a.speak(ctx, args[1:])
	case "serve":
		return a.serve(ctx, args[1:])
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
	case "jobs":
		return a.jobsCommand(ctx, args[1:])
	case "image":
		return a.mediaJob(ctx, "image-generation", args[1:])
	case "video":
		return a.mediaJob(ctx, "video-generation", args[1:])
	case "_daemon":
		return a.serve(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q; run `backpack help`", args[0])
	}
}

func (a *app) commandHelp(command string) error {
	usage := map[string]string{
		"models":     "Usage: backpack models\n\nList the trusted model catalog and support status.\n",
		"catalog":    "Usage: backpack catalog verify [--json]\n\nCompare the trusted catalog with public Backpack packages on Hugging Face.\n",
		"pull":       "Usage: backpack pull <model>\n\nResolve, download, verify, and atomically install a model package.\n",
		"list":       "Usage: backpack list\n\nList installed model packages.\n",
		"inspect":    "Usage: backpack inspect <model>\n\nInspect package metadata, runtime compatibility, and local fit without downloading weights.\n",
		"hardware":   "Usage: backpack hardware\n\nInspect local CPU, memory, GPU, and runtime capabilities.\n",
		"run":        "Usage: backpack run <model> [--prompt text] [--context tokens] [--gpu-layers auto|n] [--keep-alive] [--detach] [--force]\n",
		"transcribe": "Usage: backpack transcribe <audio-file> [--model model] [--language code] [--compute target] [--force]\n",
		"speak":      "Usage: backpack speak <text> --output file.wav [--model model] [--voice voice] [--speed n] [--compute target] [--force]\n",
		"serve":      "Usage: backpack serve [--address 127.0.0.1:port]\n\nRun the local HTTP service in the foreground.\n",
		"ps":         "Usage: backpack ps\n\nList service-owned runtime sessions.\n",
		"stop":       "Usage: backpack stop <session-id>\n\nGracefully stop one exact session.\n",
		"compute":    "Usage: backpack compute <list|add|show|test|doctor|remove> [arguments]\n",
		"runtime":    "Usage: backpack runtime <list|show|install|verify|remove> [arguments]\n",
		"doctor":     "Usage: backpack doctor [--json]\n\nPrint sanitized local diagnostics suitable for bug reports.\n",
		"jobs":       "Usage: backpack jobs <list|show|cancel> [arguments] [--json]\n",
		"image":      "Usage: backpack image <model> --prompt text [--width n] [--height n] [--steps n] [--seed n] [--compute target]\n\nExperimental: submission requires an execution-validated image runner.\n",
		"video":      "Usage: backpack video <model> --prompt text [--image path] [--frames n] [--fps n] [--steps n] [--seed n] [--compute target]\n\nExperimental: submission requires an execution-validated video runner.\n",
		"version":    "Usage: backpack version\n",
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

  models                  list the official catalog
  catalog verify          detect trusted catalog/Hugging Face drift
  pull <model>            download and verify a package
  list                    list installed packages
  inspect <model>         show manifest/runtime compatibility
  hardware                inspect local compute
  run <model> [flags]     create an API-owned session and chat
  transcribe <audio>      transcribe audio through the runtime API
  speak <text>            synthesize speech through the runtime API
  serve [--address addr]  start the loopback runtime API
  ps                      list runtime-owned sessions
  stop <session>          gracefully stop a session
  compute <command>       manage local and SSH compute targets
  runtime <command>       inspect and manage inference runtimes
  doctor [--json]         print sanitized release diagnostics
  jobs <command>          list, inspect, or cancel long-running jobs
  image <model> [flags]   submit an image-generation job (experimental)
  video <model> [flags]   submit a video-generation job (experimental)
  version

Run flags: --prompt text --context tokens --gpu-layers auto|n --keep-alive --detach --force
`)
	return nil
}

func (a *app) transcribe(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack transcribe <audio-file> [--model model] [--language code] [--compute target]")
	}
	audioPath := args[0]
	fs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
	fs.SetOutput(a.err)
	model := fs.String("model", "whisper-large-v3-turbo", "transcription model")
	language := fs.String("language", "", "language code; auto-detect when omitted")
	computeName := fs.String("compute", "local", "compute target")
	force := fs.Bool("force", false, "run even when model fit recommends remote compute")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack transcribe <audio-file> [flags]")
	}
	if _, err := os.Stat(audioPath); err != nil {
		return fmt.Errorf("audio input: %w", err)
	}
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	stopEvents := a.watchEvents(ctx, api)
	defer stopEvents()
	result, err := api.Transcribe(ctx, clientapi.TranscriptionRequest{Model: *model, AudioPath: audioPath, Language: *language, Compute: *computeName, Force: *force})
	if err != nil {
		return err
	}
	fmt.Fprintln(a.out, result.Text)
	return nil
}

func (a *app) speak(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack speak <text> --output file.wav [flags]")
	}
	input := args[0]
	fs := flag.NewFlagSet("speak", flag.ContinueOnError)
	fs.SetOutput(a.err)
	model := fs.String("model", "kokoro-82m", "speech model")
	voice := fs.String("voice", "af_heart", "voice name")
	output := fs.String("output", "", "output WAV path")
	speed := fs.Float64("speed", 1, "speech speed from 0.5 to 2.0")
	computeName := fs.String("compute", "local", "compute target")
	force := fs.Bool("force", false, "overwrite an existing output file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || *output == "" {
		return fmt.Errorf("usage: backpack speak <text> --output file.wav [flags]")
	}
	if *speed < 0.5 || *speed > 2 {
		return fmt.Errorf("--speed must be between 0.5 and 2.0")
	}
	if _, err := os.Stat(*output); err == nil && !*force {
		return fmt.Errorf("output %q already exists; use --force to overwrite it", *output)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	stopEvents := a.watchEvents(ctx, api)
	defer stopEvents()
	audio, err := api.Speech(ctx, clientapi.SpeechRequest{Model: *model, Input: input, Voice: *voice, Format: "wav", Compute: *computeName, Speed: *speed, Force: *force})
	if err != nil {
		return err
	}
	directory := filepath.Dir(*output)
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".backpack-speech-*.wav")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(audio); err == nil {
		err = temporary.Close()
	} else {
		_ = temporary.Close()
	}
	if err != nil {
		return err
	}
	if *force {
		_ = os.Remove(*output)
	}
	if err = os.Rename(temporaryPath, *output); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Wrote %s (%d bytes)\n", *output, len(audio))
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
	report := diagnostics.Collect(ctx, a.version, a.paths, a.local, a.models, a.runtimes, compute.NewTargetStore(a.paths))
	if *asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	}
	fmt.Fprintf(a.out, "Backpack %s\nPlatform: %s\nHome: %s\nDaemon: %v\nModels: %d installed, %d verified, %d corrupt\nRuntimes: %d installed, %d verified, %d corrupt\nCompute targets: %d\n", report.Version, report.Platform, report.BackpackHome, report.Daemon.Running, report.Models.Installed, report.Models.Verified, report.Models.Corrupt, report.Runtimes.Installed, report.Runtimes.Verified, report.Runtimes.Corrupt, len(report.ComputeTargets))
	for _, problem := range report.KnownProblems {
		fmt.Fprintln(a.out, "Problem:", problem)
	}
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
		fmt.Fprintf(a.out, "%d HF packages discovered\n%d catalog packages represented\n", report.Discovered, report.Represented)
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

func (a *app) jobsCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack jobs <list|show|cancel>")
	}
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("jobs list", flag.ContinueOnError)
		fs.SetOutput(a.err)
		asJSON := fs.Bool("json", false, "emit machine-readable JSON")
		if err = fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return fmt.Errorf("usage: backpack jobs list [--json]")
		}
		items, listErr := api.Jobs(ctx)
		if listErr != nil {
			return listErr
		}
		if *asJSON {
			data, _ := json.MarshalIndent(map[string]any{"data": items}, "", "  ")
			fmt.Fprintln(a.out, string(data))
			return nil
		}
		if len(items) == 0 {
			fmt.Fprintln(a.out, "No jobs.")
			return nil
		}
		fmt.Fprintln(a.out, "JOB                                  MODEL                       CAPABILITY         STATUS")
		for _, item := range items {
			fmt.Fprintf(a.out, "%-36s %-27s %-18s %s\n", item.ID, item.Model, item.Capability, item.Status)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack jobs show <job>")
		}
		item, getErr := api.GetJob(ctx, args[1])
		if getErr != nil {
			return getErr
		}
		data, _ := json.MarshalIndent(item, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	case "cancel":
		if len(args) != 2 {
			return fmt.Errorf("usage: backpack jobs cancel <job>")
		}
		if err = api.CancelJob(ctx, args[1]); err != nil {
			return err
		}
		fmt.Fprintln(a.out, "Cancellation requested", args[1])
		return nil
	default:
		return fmt.Errorf("unknown jobs command %q", args[0])
	}
}

func (a *app) mediaJob(ctx context.Context, capability string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack %s <model> --prompt <text> [flags]", strings.TrimSuffix(capability, "-generation"))
	}
	model := args[0]
	fs := flag.NewFlagSet(capability, flag.ContinueOnError)
	fs.SetOutput(a.err)
	prompt := fs.String("prompt", "", "generation prompt")
	image := fs.String("image", "", "input image for supported video models")
	width := fs.Int("width", 0, "output width")
	height := fs.Int("height", 0, "output height")
	steps := fs.Int("steps", 0, "generation steps")
	seed := fs.Int64("seed", 0, "generation seed")
	frames := fs.Int("frames", 0, "video frame count")
	fps := fs.Float64("fps", 0, "video frames per second")
	computeName := fs.String("compute", "local", "compute target")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *prompt == "" {
		return fmt.Errorf("--prompt is required")
	}
	if capability == "image-generation" && *image != "" {
		return fmt.Errorf("--image is only valid for video generation")
	}
	var seedValue *int64
	fs.Visit(func(item *flag.Flag) {
		if item.Name == "seed" {
			seedValue = seed
		}
	})
	api, err := daemon.Ensure(ctx, a.paths, a.version)
	if err != nil {
		return err
	}
	job, err := api.CreateJob(ctx, clientapi.CreateJobRequest{Model: model, Capability: capability, Compute: *computeName, Input: clientapi.JobInput{Prompt: *prompt, Image: *image}, Options: clientapi.JobOptions{Width: *width, Height: *height, Steps: *steps, Seed: seedValue, Frames: *frames, FPS: *fps}})
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(job, "", "  ")
	fmt.Fprintln(a.out, string(data))
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
	broker := events.NewBroker()
	a.runtimes.Sink = broker.Publish
	manager := sessions.NewWithEvents(a.catalog, a.models, a.registry, a.paths, broker.Publish)
	jobManager := jobs.New(a.catalog, a.paths, broker.Publish)
	s := &server.Server{Version: a.version, Catalog: a.catalog, Models: a.models, Sessions: manager, Audio: manager, Speech: manager, Jobs: jobManager, Events: broker, Targets: compute.NewTargetStore(a.paths), Hardware: func() (compute.Hardware, error) { return a.local.Inspect(ctx) }}
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
