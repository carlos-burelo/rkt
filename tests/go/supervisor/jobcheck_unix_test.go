//go:build unix

package supervisor

import (
	"strconv"
	"strings"
	"syscall"
)

func jobContains(job *jobCtx, pid uint32) bool {
	if job == nil {
		return false
	}
	job.mu.Lock()
	pgid := job.pgid
	job.mu.Unlock()
	g, err := syscall.Getpgid(int(pid))
	if err != nil {
		return false
	}
	return pgid > 0 && g == pgid
}

func jobContainsPid(job *jobCtx, s string) bool {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return jobContains(job, uint32(n))
}
