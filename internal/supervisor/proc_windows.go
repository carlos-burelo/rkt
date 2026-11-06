//go:build windows

package supervisor

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureProc evita ventana de consola, crea un grupo propio (para
// CTRL_BREAK) y evita que conhost herede los handles del log.
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000 | windows.CREATE_NEW_PROCESS_GROUP,
	}
}

// requestStop intenta un cierre ordenado (CTRL_BREAK al grupo). Si el
// servicio no tiene consola, GenerateConsoleCtrlEvent falla y el caller
// cae al kill duro.
func requestStop(pid int) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid))
}
