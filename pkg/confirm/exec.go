package confirm

import "os/exec"

// openCommand launches an external command. Extracted so tests can stub openBrowser.
func openCommand(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
