//go:build windows

package supervisor

import (
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procIsProcessInJob = modkernel32.NewProc("IsProcessInJob")
)

func jobContains(job *jobCtx, pid uint32) bool {
	if job == nil {
		return false
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(ph)
	var inJob bool
	r1, _, e1 := procIsProcessInJob.Call(uintptr(ph), uintptr(job.h), uintptr(unsafe.Pointer(&inJob)))
	if r1 == 0 {
		return false
	}
	_ = e1
	return inJob
}

func jobContainsPid(job *jobCtx, s string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return jobContains(job, uint32(n))
}
