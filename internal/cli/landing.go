package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

func (a *app) landing(ctx context.Context) error {
	if !terminalInput(os.Stdin) {
		return a.help()
	}
	fmt.Fprint(a.out, `Backpack Runtime

  1  Run a model
  2  Browse models
  3  Launch a coding agent (experimental)
  4  Run doctor
  5  List compute targets
  6  Show help
  0  Exit

Choose: `)
	reader := bufio.NewReader(os.Stdin)
	choice, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(choice) == "" {
		return err
	}
	switch strings.TrimSpace(choice) {
	case "1", "run":
		fmt.Fprint(a.out, "Model [smollm2-135m]: ")
		model, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(model) == "" {
			return readErr
		}
		model = strings.TrimSpace(model)
		if model == "" {
			model = "smollm2-135m"
		}
		return a.run(ctx, []string{model})
	case "2", "models":
		return a.catalogList(nil)
	case "3", "launch":
		fmt.Fprint(a.out, "Agent [codex]: ")
		agent, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(agent) == "" {
			return readErr
		}
		agent = strings.TrimSpace(agent)
		if agent == "" {
			agent = "codex"
		}
		return a.launchCommand(ctx, []string{agent})
	case "4", "doctor":
		return a.doctor(ctx, nil)
	case "5", "compute":
		return a.compute(ctx, []string{"list"})
	case "6", "help":
		return a.help()
	case "0", "exit", "quit":
		return nil
	default:
		return fmt.Errorf("unknown selection %q; run `backpack help`", strings.TrimSpace(choice))
	}
}

func terminalInput(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
