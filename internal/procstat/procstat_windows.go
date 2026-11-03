//go:build windows

package procstat

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type memoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

var (
	psapi          = windows.NewLazySystemDLL("psapi.dll")
	procMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
)

// Read abre el proceso y devuelve RSS + CPU acumulada.
func Read(pid int) (Sample, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return Sample{}, fmt.Errorf("abrir pid %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var mc memoryCounters
	mc.Cb = uint32(unsafe.Sizeof(mc))
	if r, _, err := procMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.Cb)); r == 0 {
		return Sample{}, fmt.Errorf("GetProcessMemoryInfo: %w", err)
	}

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return Sample{}, fmt.Errorf("GetProcessTimes: %w", err)
	}
	cpu := filetimeDuration(kernel) + filetimeDuration(user)
	return Sample{RSSBytes: uint64(mc.WorkingSetSize), CPUTime: cpu}, nil
}

func filetimeDuration(ft windows.Filetime) time.Duration {
	// 100ns ticks
	ticks := uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
	return time.Duration(ticks * 100)
}
