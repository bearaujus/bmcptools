//go:build windows

package main

import "os/exec"

// setSysProcDetach is a no-op on Windows; the process is started without
// additional session isolation.
func setSysProcDetach(_ *exec.Cmd) {}
