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

  1  Run a coding workspace
  2  Browse models
  3  Run doctor
  4  List compute targets
  5  Show help
  0  Exit

Choose: `)
	reader := bufio.NewReader(os.Stdin)
	choice, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(choice) == "" {
		return err
	}
	switch strings.TrimSpace(choice) {
	case "1", "run":
		fmt.Fprint(a.out, "App [codex]: ")
		workspace, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(workspace) == "" {
			return readErr
		}
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			workspace = "codex"
		}
		return a.runCommand(ctx, []string{workspace})
	case "2", "models":
		return a.catalogList(nil)
	case "3", "doctor":
		return a.doctor(ctx, nil)
	case "4", "compute":
		return a.compute(ctx, []string{"list"})
	case "5", "help":
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
