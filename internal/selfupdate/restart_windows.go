//go:build windows

package selfupdate

import (
	"os/exec"
	"syscall"
)

// Restart запускает exe заново отдельным процессом (не привязанным к текущему окну и консоли).
func Restart(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000008 | 0x00000200} // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	return cmd.Start()
}
