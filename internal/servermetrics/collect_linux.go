//go:build linux

package servermetrics

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func readPlatform() (rawSample, error) {
	host, _ := os.Hostname()
	out := rawSample{hostname: host, os: runtime.GOOS, warnings: []string{}}

	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return out, err
	}
	cpu, cores, err := parseProcStat(string(stat))
	if err != nil {
		return out, err
	}
	out.cpu, out.perCore, out.cores = cpu, cores, len(cores)

	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		out.memory = parseMeminfo(string(b))
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		out.load = parseLoadavg(string(b))
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		f := strings.Fields(string(b))
		if len(f) > 0 {
			v, _ := strconv.ParseFloat(f[0], 64)
			out.uptimeS = int64(v)
		}
	}
	if b, err := os.ReadFile("/proc/net/dev"); err == nil {
		out.rx, out.tx = parseNetDev(string(b))
	}
	out.disks = linuxDisks()
	out.temps = linuxTemps()
	out.firewall = linuxFirewall()
	return out, nil
}

func linuxDisks() []Disk {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := []Disk{}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		mount, fs := f[1], f[2]
		if seen[mount] || !realFS(fs) {
			continue
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(mount, &st); err != nil || st.Blocks == 0 {
			continue
		}
		seen[mount] = true
		total := st.Blocks * uint64(st.Bsize)
		free := st.Bavail * uint64(st.Bsize)
		used := total - free
		out = append(out, Disk{
			Mount: mount, FSType: fs, Total: total, Free: free, Used: used,
			Percent: float64(used) / float64(total) * 100,
		})
	}
	SortDisks(out)
	return out
}

func realFS(fs string) bool {
	switch fs {
	case "ext2", "ext3", "ext4", "xfs", "btrfs", "zfs", "f2fs", "vfat", "exfat", "ntfs", "ntfs3", "overlay":
		return true
	}
	return false
}

func linuxTemps() []Temp {
	out := []Temp{}
	paths, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil {
			continue
		}
		if v > 1000 {
			v /= 1000
		}
		zone := filepath.Base(filepath.Dir(p))
		if label, err := os.ReadFile(filepath.Join(filepath.Dir(p), "type")); err == nil {
			if s := strings.TrimSpace(string(label)); s != "" {
				zone = s
			}
		}
		out = append(out, Temp{Zone: zone, Celsius: v})
	}
	return out
}

func linuxFirewall() Firewall {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, args := range [][]string{{"nft", "list", "ruleset"}, {"iptables", "-S"}} {
		out, err := exec.CommandContext(ctx, args[0], args[1:]...).Output()
		if err == nil {
			text := strings.TrimSpace(string(out))
			return Firewall{Available: true, Enabled: text != "", Detail: firstLine(text)}
		}
	}
	return Firewall{Available: false, Detail: "nft/iptables no legibles"}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func listProcesses() []procEntry {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := []procEntry{}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		name, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		out = append(out, procEntry{pid: pid, name: strings.TrimSpace(string(name))})
	}
	return out
}
