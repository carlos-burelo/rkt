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

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type threadEntry struct {
	Size           uint32
	CntUsage       uint32
	ThreadID       uint32
	OwnerProcessID uint32
	BasePri        int32
	DeltaPri       int32
	Flags          uint32
}

var (
	psapi          = windows.NewLazySystemDLL("psapi.dll")
	kernel32       = windows.NewLazySystemDLL("kernel32.dll")
	procMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
	procIOCounters = kernel32.NewProc("GetProcessIoCounters")
	procHandleCnt  = kernel32.NewProc("GetProcessHandleCount")
	procSnapshot   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procThread1st  = kernel32.NewProc("Thread32First")
	procThreadNext = kernel32.NewProc("Thread32Next")
)

const (
	processQueryLimited = windows.PROCESS_QUERY_LIMITED_INFORMATION
	processVMRead       = windows.PROCESS_VM_READ
	snapThread          = 0x00000004
	invalidHandle       = ^uintptr(0)
)

// Read abre el proceso y devuelve memoria, CPU acumulada, I/O y handles.
func Read(pid int) (Sample, error) {
	h, err := windows.OpenProcess(processQueryLimited|processVMRead, false, uint32(pid))
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

	var io ioCounters
	_, _, _ = procIOCounters.Call(uintptr(h), uintptr(unsafe.Pointer(&io)))
	var handles uint32
	_, _, _ = procHandleCnt.Call(uintptr(h), uintptr(unsafe.Pointer(&handles)))

	return Sample{
		RSSBytes:     uint64(mc.WorkingSetSize),
		PeakRSSBytes: uint64(mc.PeakWorkingSetSize),
		VirtualBytes: uint64(mc.PagefileUsage),
		CPUTime:      cpu,
		ReadBytes:    io.ReadTransferCount,
		WriteBytes:   io.WriteTransferCount,
		Handles:      handles,
		Threads:      countThreads(uint32(pid)),
		StartTime:    filetimeTime(creation),
	}, nil
}

// filetimeTime convierte FILETIME (1601) a time.Time.
func filetimeTime(ft windows.Filetime) time.Time {
	ticks := uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
	const epochDiff = 116444736000000000 // 100ns entre 1601 y 1970
	if ticks < epochDiff {
		return time.Time{}
	}
	return time.Unix(0, int64(ticks-epochDiff)*100)
}

func countThreads(pid uint32) uint32 {
	snap, _, _ := procSnapshot.Call(snapThread, 0)
	if snap == 0 || snap == invalidHandle {
		return 0
	}
	defer windows.CloseHandle(windows.Handle(snap))
	var te threadEntry
	te.Size = uint32(unsafe.Sizeof(te))
	var n uint32
	for ok, _, _ := procThread1st.Call(snap, uintptr(unsafe.Pointer(&te))); ok != 0; ok, _, _ = procThreadNext.Call(snap, uintptr(unsafe.Pointer(&te))) {
		if te.OwnerProcessID == pid {
			n++
		}
	}
	return n
}

func filetimeDuration(ft windows.Filetime) time.Duration {
	// 100ns ticks
	ticks := uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
	return time.Duration(ticks * 100)
}
