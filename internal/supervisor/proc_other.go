//go:build !windows

package supervisor

import "os/exec"

// configureProc sin-op fuera de Windows.
func configureProc(cmd *exec.Cmd) {}
