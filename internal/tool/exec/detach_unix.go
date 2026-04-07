//go:build !windows

package exec

import (
	"os/exec"
	"syscall"
)

func setSysProcDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
