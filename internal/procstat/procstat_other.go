//go:build !windows

package procstat

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Read RSS + CPU desde /proc (Linux/macOS con procfs asumido Linux).
func Read(pid int) (Sample, error) {
	statm, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return Sample{}, err
	}
	fields := strings.Fields(string(statm))
	if len(fields) < 2 {
		return Sample{}, fmt.Errorf("statm inválido")
	}
	pages, _ := strconv.ParseUint(fields[1], 10, 64)
	pageSize := uint64(os.Getpagesize())

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Sample{}, err
	}
	parts := strings.Fields(string(stat))
	var cpu time.Duration
	if len(parts) > 15 {
		ut, _ := strconv.ParseUint(parts[13], 10, 64)
		st, _ := strconv.ParseUint(parts[14], 10, 64)
		cpu = time.Duration(ut+st) * (time.Second / 100)
	}
	return Sample{RSSBytes: pages * pageSize, CPUTime: cpu}, nil
}
