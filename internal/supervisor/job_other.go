//go:build !windows

package supervisor

// jobCtx sin-op fuera de Windows (sin Job Objects ni tope RAM).
type jobCtx struct{}

func newJob(memoryMB uint64) (*jobCtx, error) { return &jobCtx{}, nil }

func (j *jobCtx) Assign(pid uint32) error { return nil }

func (j *jobCtx) Close() {}
