//go:build unix

package supervisor

import (
	"sync"
	"syscall"
)

// jobCtx en Unix representa el grupo de procesos de la app. Sin Job Object
// no hay límite de RAM; sí kill del árbol completo vía process group.
type jobCtx struct {
	mu   sync.Mutex
	pgid int
}

func newJob(uint64) (*jobCtx, error) { return &jobCtx{}, nil }

func (j *jobCtx) Assign(pid uint32) error {
	j.mu.Lock()
	j.pgid = int(pid)
	j.mu.Unlock()
	return nil
}

// Kill mata el grupo entero (padre + hijos). Setpgid en configureProc
// garantiza que el pgid sea el pid del proceso lanzado.
func (j *jobCtx) Kill() {
	j.mu.Lock()
	pgid := j.pgid
	j.mu.Unlock()
	if pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}

func (j *jobCtx) Close() {}
