//go:build windows

package canary

import (
	"os/exec"
	"syscall"
)

// detach: proceso nuevo, sin consola y fuera del grupo del servicio.
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
		HideWindow:    true,
	}
}
