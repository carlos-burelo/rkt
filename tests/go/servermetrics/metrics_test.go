package servermetrics

import (
	"testing"
	"time"
)

func TestParseProcStat(t *testing.T) {
	data := "cpu  10 2 3 80 5 0 0 0 0 0\ncpu0 5 1 2 40 2 0 0 0 0 0\ncpu1 5 1 1 40 3 0 0 0 0 0\n"
	total, cores, err := parseProcStat(data)
	if err != nil {
		t.Fatal(err)
	}
	if total.idle != 85 || len(cores) != 2 || cores[0].total != 50 {
		t.Fatalf("total=%+v cores=%+v", total, cores)
	}
	if _, _, err := parseProcStat("sin cpu"); err == nil {
		t.Fatal("sin cpu debe fallar")
	}
}

func TestParseMeminfo(t *testing.T) {
	data := "MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB\nCached: 200 kB\nSReclaimable: 50 kB\nSwapTotal: 500 kB\nSwapFree: 300 kB\n"
	m := parseMeminfo(data)
	if m.Total != 1000*1024 || m.Used != 600*1024 || m.Cached != 250*1024 || m.SwapUsed != 200*1024 {
		t.Fatalf("mem = %+v", m)
	}
	if m.Percent != 60 {
		t.Fatalf("percent = %v", m.Percent)
	}
	// sin MemAvailable se aproxima con free+buffers+cached
	m2 := parseMeminfo("MemTotal: 100 kB\nMemFree: 10 kB\nCached: 20 kB\n")
	if m2.Available != 30*1024 {
		t.Fatalf("available = %d", m2.Available)
	}
}

func TestParseLoadAndNet(t *testing.T) {
	l := parseLoadavg("1.20 0.50 0.25 1/100 123")
	if !l.Supported || l.L1 != 1.2 || l.L15 != 0.25 {
		t.Fatalf("load = %+v", l)
	}
	if parseLoadavg("x").Supported {
		t.Fatal("load inválido")
	}
	rx, tx := parseNetDev("lo: 1 2 3 4\neth0: 100 2 3 4 5 6 7 8 900 10\n")
	if rx != 100 || tx != 900 {
		t.Fatalf("net = %d/%d", rx, tx)
	}
}

func TestCPUPercentYRate(t *testing.T) {
	if p := cpuPercent(cpuTimes{total: 100, idle: 50}, cpuTimes{total: 200, idle: 120}); p != 30 {
		t.Fatalf("percent = %v", p)
	}
	if p := cpuPercent(cpuTimes{total: 0, idle: 0}, cpuTimes{total: 0, idle: 0}); p != 0 {
		t.Fatalf("sin delta = %v", p)
	}
	if r := rate(100, 300, 2); r != 100 {
		t.Fatalf("rate = %v", r)
	}
	if r := rate(300, 100, 2); r != 0 {
		t.Fatalf("rate reset = %v", r)
	}
}

func TestHistoryYPoint(t *testing.T) {
	now := time.Now().UTC()
	m := &Monitor{
		history: []HistoryPoint{{Ts: now.Format(time.RFC3339), CPU: 10}, {Ts: now.Add(-48 * time.Hour).Format(time.RFC3339)}},
	}
	if got := m.History(6); len(got) != 1 {
		t.Fatalf("history = %+v", got)
	}
	if got := m.History(99); len(got) != 1 {
		t.Fatalf("history tope = %+v", got)
	}
	s := Snapshot{
		Ts: now.Format(time.RFC3339), CPU: CPU{Percent: 20},
		Memory:  Memory{Percent: 30},
		Disks:   []Disk{{Percent: 40}, {Percent: 60}},
		Temps:   []Temp{{Celsius: 50}, {Celsius: 70}},
		Network: Network{RxRate: 1, TxRate: 2}, Load: Load{L1: 0.5},
	}
	p := pointOf(s)
	if p.Disk != 60 || p.TempMax != 70 || p.CPU != 20 {
		t.Fatalf("point = %+v", p)
	}
}
