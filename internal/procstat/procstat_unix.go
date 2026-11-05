//go:build unix

package procstat

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// readKVM convierte "VmRSS:  123 kB" a bytes. 0 si no está.
func readKVMBytes(status, key string) uint64 {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		n, _ := strconv.ParseUint(fields[1], 10, 64)
		return n * 1024
	}
	return 0
}

func readUint(status, key string) uint64 {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		n, _ := strconv.ParseUint(fields[1], 10, 64)
		return n
	}
	return 0
}

// bootTime segundos Unix desde /proc/stat (btime).
func bootTime() int64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
		return n
	}
	return 0
}

// Read lee /proc/<pid>/{stat,status,io}. En SO Unix sin /proc devuelve error.
func Read(pid int) (Sample, error) {
	statRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Sample{}, fmt.Errorf("leer pid %d: %w", pid, err)
	}
	// El nombre del proceso puede llevar espacios y paréntesis: se corta
	// por el último ')' y los campos siguientes mantienen su orden.
	s := string(statRaw)
	end := strings.LastIndexByte(s, ')')
	if end < 0 || end+2 >= len(s) {
		return Sample{}, fmt.Errorf("stat de pid %d inválido", pid)
	}
	f := strings.Fields(s[end+2:])
	if len(f) < 22 {
		return Sample{}, fmt.Errorf("stat de pid %d incompleto", pid)
	}
	utime, _ := strconv.ParseUint(f[11], 10, 64)
	stime, _ := strconv.ParseUint(f[12], 10, 64)
	threads, _ := strconv.ParseUint(f[17], 10, 64)
	startTicks, _ := strconv.ParseUint(f[19], 10, 64)
	vsize, _ := strconv.ParseUint(f[20], 10, 64)
	rssPages, _ := strconv.ParseUint(f[21], 10, 64)

	status, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	statusS := string(status)
	rss := readKVMBytes(statusS, "VmRSS")
	if rss == 0 {
		rss = rssPages * uint64(os.Getpagesize())
	}
	peak := readKVMBytes(statusS, "VmHWM")
	if threads == 0 {
		threads = readUint(statusS, "Threads")
	}

	var readBytes, writeBytes uint64
	if ioRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/io", pid)); err == nil {
		ioS := string(ioRaw)
		readBytes = readUint(ioS, "read_bytes")
		writeBytes = readUint(ioS, "write_bytes")
	}

	// 100 Hz es el USER_HZ estándar de Linux.
	cpu := time.Duration(utime+stime) * time.Second / 100
	start := time.Time{}
	if b := bootTime(); b > 0 {
		start = time.Unix(b, 0).Add(time.Duration(startTicks) * time.Second / 100)
	}
	handles := uint32(0)
	if entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid)); err == nil {
		handles = uint32(len(entries))
	}

	return Sample{
		RSSBytes:     rss,
		PeakRSSBytes: peak,
		VirtualBytes: vsize,
		CPUTime:      cpu,
		ReadBytes:    readBytes,
		WriteBytes:   writeBytes,
		Handles:      handles,
		Threads:      uint32(threads),
		StartTime:    start,
	}, nil
}
