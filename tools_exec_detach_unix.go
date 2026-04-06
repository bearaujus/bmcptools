//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setSysProcDetach configures cmd to start in a new session (setsid),
// fully detaching it from the parent process group and terminal.
func setSysProcDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
