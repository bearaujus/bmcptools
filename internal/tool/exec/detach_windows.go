//go:build windows

package exec

import "os/exec"

func setSysProcDetach(_ *exec.Cmd) {}
