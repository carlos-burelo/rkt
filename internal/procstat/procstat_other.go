//go:build !windows

package procstat

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Read RSS/CPU/I/O/handles desde /proc (Linux).
func Read(pid int) (Sample, error) {
	statm, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return Sample{}, err
	}
	fields := strings.Fields(string(statm))
	if len(fields) < 7 {
		return Sample{}, fmt.Errorf("statm inválido")
	}
	pageSize := uint64(os.Getpagesize())
	pages, _ := strconv.ParseUint(fields[1], 10, 64)
	virtual, _ := strconv.ParseUint(fields[0], 10, 64)

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Sample{}, err
	}
	parts := strings.Fields(string(stat))
	var cpu time.Duration
	var threads uint32
	if len(parts) > 19 {
		ut, _ := strconv.ParseUint(parts[13], 10, 64)
		st, _ := strconv.ParseUint(parts[14], 10, 64)
		cpu = time.Duration(ut+st) * (time.Second / 100)
		t, _ := strconv.ParseUint(parts[19], 10, 32)
		threads = uint32(t)
	}

	var read, write uint64
	if ioRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid)); err == nil {
		for _, line := range strings.Split(string(ioRaw), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), ": "); ok {
				n, _ := strconv.ParseUint(v, 10, 64)
				switch k {
				case "read_bytes":
					read = n
				case "write_bytes":
					write = n
				}
			}
		}
	}

	var handles uint32
	if fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid)); err == nil {
		handles = uint32(len(fds))
	}

	return Sample{
		RSSBytes:     pages * pageSize,
		VirtualBytes: virtual * pageSize,
		CPUTime:      cpu,
		ReadBytes:    read,
		WriteBytes:   write,
		Handles:      handles,
		Threads:      threads,
		StartTime:    procStart(pid, parts),
	}, nil
}

// procStart combina starttime (ticks desde boot) con btime de /proc/stat.
func procStart(pid int, parts []string) time.Time {
	if len(parts) < 22 {
		return time.Time{}
	}
	ticks, _ := strconv.ParseUint(parts[21], 10, 64)
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	var btime int64
	for _, line := range strings.Split(string(stat), "\n") {
		if strings.HasPrefix(line, "btime ") {
			btime, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			break
		}
	}
	if btime == 0 {
		return time.Time{}
	}
	hz := int64(100) // USER_HZ estándar en Linux
	return time.Unix(btime+int64(ticks)/hz, 0)
}
