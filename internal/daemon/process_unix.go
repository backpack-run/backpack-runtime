//go:build !windows

package daemon

import (
	"os/exec"
	"syscall"
)

func configureBackground(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
