package user

import "os/exec"

// exec_command_run runs a command and returns its error (used by openBrowser).
func exec_command_run(name string, arg ...string) error {
	return exec.Command(name, arg...).Run()
}

// exec_command_run_windows opens a URL with Windows rundll32.
func exec_command_run_windows(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
}
