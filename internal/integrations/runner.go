package integrations

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type ProcessIO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Run starts the real third-party CLI without a shell. Backpack supplies only
// provider/model routing; the child retains its own sandbox and approval UI.
func Run(ctx context.Context, invocation Invocation, processIO ProcessIO) error {
	if err := invocation.Validate(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, invocation.Executable, invocation.Args()...)
	// Do not let descendant processes that inherited stdout/stderr keep a
	// cancelled launch stuck in Wait indefinitely.
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = invocation.Environment.Apply(os.Environ())
	cmd.Stdin = processIO.Stdin
	cmd.Stdout = processIO.Stdout
	cmd.Stderr = processIO.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launch integration process: %w", err)
	}
	return nil
}
