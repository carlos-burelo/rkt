//go:build unix

package supervisor

import (
	"os/exec"
	"syscall"
)

// configureProc crea un process group propio: matar el grupo arrastra a
// los hijos (equivalente Unix del Job Object de Windows).
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// requestStop pide cierre ordenado al grupo (SIGTERM).
func requestStop(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}
