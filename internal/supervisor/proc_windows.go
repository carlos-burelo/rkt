//go:build windows

package supervisor

import (
	"os/exec"
	"syscall"
)

// configureProc evita ventana de consola y que conhost herede (y retenga)
// los handles del log: sin esto, conhost vivo bloquea rotar/borrar el log.
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
