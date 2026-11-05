//go:build windows

package servermetrics

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
)

type filetime struct {
	Low  uint32
	High uint32
}

func (f filetime) uint64() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func readPlatform() (rawSample, error) {
	host, _ := os.Hostname()
	out := rawSample{hostname: host, os: runtime.GOOS, warnings: []string{}}

	var idle, kern, user filetime
	if r, _, err := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&user))); r == 0 {
		return out, err
	}
	out.cpu = cpuTimes{total: kern.uint64() + user.uint64(), idle: idle.uint64()}
	cores := int(windows.GetActiveProcessorCount(0xffff))
	if cores > 0 {
		out.cores = cores
	}

	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r == 0 {
		return out, err
	}
	out.memory = Memory{
		Total: ms.TotalPhys, Available: ms.AvailPhys,
		Used:      ms.TotalPhys - ms.AvailPhys,
		SwapTotal: ms.TotalPageFile, SwapUsed: ms.TotalPageFile - ms.AvailPageFile,
		Percent: float64(ms.MemoryLoad),
	}
	if ms.TotalPhys > 0 {
		out.memory.Percent = float64(ms.TotalPhys-ms.AvailPhys) / float64(ms.TotalPhys) * 100
	}
	if tick, _, _ := procGetTickCount64.Call(); tick > 0 {
		out.uptimeS = int64(tick / 1000)
	}
	out.load = Load{Supported: false}
	out.disks = windowsDisks()
	out.rx, out.tx = windowsNet()
	out.temps = []Temp{}
	out.firewall = windowsFirewall()
	return out, nil
}

func windowsDisks() []Disk {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	out := []Disk{}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		p, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		var freeAvail, total, free uint64
		if err := windows.GetDiskFreeSpaceEx(p, &freeAvail, &total, &free); err != nil || total == 0 {
			continue
		}
		used := total - free
		out = append(out, Disk{
			Mount: root, Total: total, Free: free, Used: used,
			Percent: float64(used) / float64(total) * 100,
		})
	}
	SortDisks(out)
	return out
}

func windowsNet() (uint64, uint64) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(0, &table); err != nil || table == nil {
		return 0, 0
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	var rx, tx uint64
	rows := (*[1 << 20]windows.MibIfRow2)(unsafe.Pointer(&table.Table[0]))[:table.NumEntries:table.NumEntries]
	for _, r := range rows {
		if r.OperStatus != windows.IfOperStatusUp {
			continue
		}
		rx += r.InOctets
		tx += r.OutOctets
	}
	return rx, tx
}

func windowsFirewall() Firewall {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "netsh", "advfirewall", "show", "allprofiles", "state")
	out, err := cmd.Output()
	if err != nil {
		return Firewall{Available: false, Detail: err.Error()}
	}
	fw := Firewall{Available: true, Profiles: []string{}}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.LastIndex(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		state := strings.ToUpper(strings.TrimSpace(line[colon+1:]))
		if state == "ON" || state == "OFF" {
			fw.Profiles = append(fw.Profiles, name+" "+state)
			if state == "ON" {
				fw.Enabled = true
			}
		}
	}
	return fw
}

func listProcesses() []procEntry {
	h, err := windows.CreateToolhelp32Snapshot(0x00000002, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	out := []procEntry{}
	if err := windows.Process32First(h, &e); err != nil {
		return out
	}
	for {
		name := windows.UTF16ToString(e.ExeFile[:])
		if e.ProcessID > 0 && name != "" {
			out = append(out, procEntry{pid: int(e.ProcessID), name: name})
		}
		if err := windows.Process32Next(h, &e); err != nil {
			break
		}
	}
	return out
}
