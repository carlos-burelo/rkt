package servermetrics

import (
	"fmt"
	"strconv"
	"strings"
)

// parseProcStat lee /proc/stat: cpu total e idle por núcleo.
func parseProcStat(data string) (cpuTimes, []cpuTimes, error) {
	var total cpuTimes
	cores := []cpuTimes{}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		vals := make([]uint64, 0, len(f)-1)
		for _, v := range f[1:] {
			n, _ := strconv.ParseUint(v, 10, 64)
			vals = append(vals, n)
		}
		if len(vals) < 4 {
			continue
		}
		// user nice system idle iowait irq softirq steal
		busy := vals[0] + vals[1] + vals[2]
		idle := vals[3]
		if len(vals) > 4 {
			idle += vals[4]
		}
		if len(vals) > 6 {
			busy += vals[5] + vals[6]
		}
		if len(vals) > 7 {
			busy += vals[7]
		}
		t := cpuTimes{total: busy + idle, idle: idle}
		if f[0] == "cpu" {
			total = t
		} else {
			cores = append(cores, t)
		}
	}
	if total.total == 0 {
		return total, cores, fmt.Errorf("proc/stat sin cpu")
	}
	return total, cores, nil
}

func parseMeminfo(data string) Memory {
	m := map[string]uint64{}
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.ParseUint(f[1], 10, 64)
		m[strings.TrimSuffix(f[0], ":")] = n * 1024
	}
	out := Memory{
		Total: m["MemTotal"], Available: m["MemAvailable"],
		Cached: m["Cached"] + m["SReclaimable"],
	}
	if out.Available == 0 {
		out.Available = m["MemFree"] + m["Buffers"] + out.Cached
	}
	out.Used = out.Total - out.Available
	out.SwapTotal = m["SwapTotal"]
	out.SwapUsed = m["SwapTotal"] - m["SwapFree"]
	if out.Total > 0 {
		out.Percent = float64(out.Used) / float64(out.Total) * 100
	}
	return out
}

func parseLoadavg(data string) Load {
	f := strings.Fields(data)
	if len(f) < 3 {
		return Load{}
	}
	l1, _ := strconv.ParseFloat(f[0], 64)
	l5, _ := strconv.ParseFloat(f[1], 64)
	l15, _ := strconv.ParseFloat(f[2], 64)
	return Load{L1: l1, L5: l5, L15: l15, Supported: true}
}

func parseNetDev(data string) (uint64, uint64) {
	var rx, tx uint64
	for _, line := range strings.Split(data, "\n") {
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "lo" || strings.HasPrefix(name, "lo:") {
			continue
		}
		f := strings.Fields(line[colon+1:])
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx += r
		tx += t
	}
	return rx, tx
}
