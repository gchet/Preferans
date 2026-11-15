//go:build !windows

package selfupdate

import "os/exec"

// Restart запускает exe заново отдельным процессом.
func Restart(exe string, args []string) error {
	return exec.Command(exe, args...).Start()
}
