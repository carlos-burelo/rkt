//go:build windows

package supervisor

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobCtx es un Job Object por app: mata los hijos con el padre
// (KILL_ON_JOB_CLOSE) y opcionalmente topa RAM por proceso.
type jobCtx struct {
	h windows.Handle
}

func newJob(memoryMB uint64) (*jobCtx, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if memoryMB > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
		info.ProcessMemoryLimit = uintptr(memoryMB * 1024 * 1024)
	}
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &jobCtx{h: h}, nil
}

// Assign mete el pid al job. Si el proceso ya está en otro job
// (rkt anidado) falla: el llamador degrada sin límites.
func (j *jobCtx) Assign(pid uint32) error {
	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE
	ph, err := windows.OpenProcess(access, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ph)
	return windows.AssignProcessToJobObject(j.h, ph)
}

// Kill termina todos los procesos del job (padre + hijos).
func (j *jobCtx) Kill() {
	if j != nil {
		_ = windows.TerminateJobObject(j.h, 1)
	}
}

// Close libera el job (con KILL_ON_JOB_CLOSE, mata lo que quede).
func (j *jobCtx) Close() {
	if j != nil {
		windows.CloseHandle(j.h)
	}
}
