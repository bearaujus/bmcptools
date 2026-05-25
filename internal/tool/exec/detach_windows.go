//go:build windows

package exec

import (
	osexec "os/exec"
	"strconv"
	"syscall"
	"time"
)

func setSysProcDetach(_ *osexec.Cmd) {}

func configureTimeoutCommand(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := osexec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
}
