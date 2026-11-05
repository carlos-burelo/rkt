// Package servermetrics mide la instancia local donde corre rkt: CPU, memoria,
// disco, red, load, uptime, temperatura y firewall. Sin agentes remotos.
package servermetrics

import (
	"sort"
	"sync"
	"time"

	"rkt-proxy/internal/procstat"
)

// Snapshot foto completa del servidor.
type Snapshot struct {
	Ts       string   `json:"ts"`
	Hostname string   `json:"hostname"`
	OS       string   `json:"os"`
	UptimeS  int64    `json:"uptime_s"`
	CPU      CPU      `json:"cpu"`
	Load     Load     `json:"load"`
	Memory   Memory   `json:"memory"`
	Disks    []Disk   `json:"disks"`
	Network  Network  `json:"network"`
	Temps    []Temp   `json:"temps"`
	Firewall Firewall `json:"firewall"`
	Warnings []string `json:"warnings,omitempty"`
}

// CPU uso agregado y por núcleo.
type CPU struct {
	Percent float64   `json:"percent"`
	Cores   int       `json:"cores"`
	PerCore []float64 `json:"per_core,omitempty"`
}

// Load average; en Windows no existe y Supported=false.
type Load struct {
	L1        float64 `json:"l1"`
	L5        float64 `json:"l5"`
	L15       float64 `json:"l15"`
	Supported bool    `json:"supported"`
}

// Memory bytes.
type Memory struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Cached    uint64  `json:"cached,omitempty"`
	SwapTotal uint64  `json:"swap_total"`
	SwapUsed  uint64  `json:"swap_used"`
	Percent   float64 `json:"percent"`
}

// Disk uso por punto de montaje.
type Disk struct {
	Mount   string  `json:"mount"`
	FSType  string  `json:"fs_type,omitempty"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

// Network acumulado y tasa desde la muestra anterior.
type Network struct {
	RxBytes uint64  `json:"rx_bytes"`
	TxBytes uint64  `json:"tx_bytes"`
	RxRate  float64 `json:"rx_rate"`
	TxRate  float64 `json:"tx_rate"`
}

// Temp sensor térmico; si no hay, Temps queda vacío.
type Temp struct {
	Zone    string  `json:"zone"`
	Celsius float64 `json:"celsius"`
}

// Firewall estado de solo lectura.
type Firewall struct {
	Available bool     `json:"available"`
	Enabled   bool     `json:"enabled"`
	Profiles  []string `json:"profiles,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

// HistoryPoint muestra compacta para la serie.
type HistoryPoint struct {
	Ts      string  `json:"ts"`
	CPU     float64 `json:"cpu"`
	Memory  float64 `json:"mem"`
	Disk    float64 `json:"disk"`
	RxRate  float64 `json:"rx_rate"`
	TxRate  float64 `json:"tx_rate"`
	Load1   float64 `json:"load1"`
	TempMax float64 `json:"temp_max,omitempty"`
}

type cpuTimes struct {
	total uint64
	idle  uint64
}

type rawSample struct {
	hostname string
	os       string
	uptimeS  int64
	cores    int
	cpu      cpuTimes
	perCore  []cpuTimes
	load     Load
	memory   Memory
	disks    []Disk
	rx, tx   uint64
	temps    []Temp
	firewall Firewall
	warnings []string
}

// Collector conserva la muestra anterior para calcular porcentajes y tasas.
type Collector struct {
	mu       sync.Mutex
	prev     *rawSample
	at       time.Time
	now      func() time.Time
	prevProc map[int]procSample
	prevAt   time.Time
	procAt   time.Time
	procTop  []Process
}

type procSample struct {
	cpu time.Duration
	at  time.Time
}

type procEntry struct {
	pid  int
	name string
}

// Process proceso de la instancia (top por CPU/RAM).
type Process struct {
	PID     int     `json:"pid"`
	Name    string  `json:"name"`
	RSSMB   float64 `json:"rss_mb"`
	CPUPct  float64 `json:"cpu_pct"`
	Threads uint32  `json:"threads,omitempty"`
}

// NewCollector crea un colector con reloj real.
func NewCollector() *Collector {
	return &Collector{now: time.Now}
}

// WithClock permite tests deterministas.
func NewCollectorWithClock(now func() time.Time) *Collector {
	return &Collector{now: now}
}

// Snapshot lee la instancia y calcula deltas contra la muestra anterior.
func (c *Collector) Snapshot() (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, err := readPlatform()
	if err != nil {
		return Snapshot{}, err
	}
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	out := Snapshot{
		Ts: now.UTC().Format(time.RFC3339), Hostname: raw.hostname, OS: raw.os,
		UptimeS: raw.uptimeS, Load: raw.load, Memory: raw.memory,
		Disks: raw.disks, Temps: raw.temps, Firewall: raw.firewall, Warnings: raw.warnings,
	}
	out.CPU.Cores = raw.cores
	if out.CPU.Cores == 0 {
		out.CPU.Cores = len(raw.perCore)
	}
	if c.prev != nil {
		out.CPU.Percent = cpuPercent(c.prev.cpu, raw.cpu)
		out.CPU.PerCore = make([]float64, 0, len(raw.perCore))
		for i := range raw.perCore {
			if i < len(c.prev.perCore) {
				out.CPU.PerCore = append(out.CPU.PerCore, cpuPercent(c.prev.perCore[i], raw.perCore[i]))
			}
		}
		elapsed := now.Sub(c.at).Seconds()
		if elapsed > 0 {
			out.Network.RxRate = rate(c.prev.rx, raw.rx, elapsed)
			out.Network.TxRate = rate(c.prev.tx, raw.tx, elapsed)
		}
	}
	out.Network.RxBytes = raw.rx
	out.Network.TxBytes = raw.tx
	c.prev = &raw
	c.at = now
	return out, nil
}

func cpuPercent(prev, cur cpuTimes) float64 {
	dt := cur.total - prev.total
	di := cur.idle - prev.idle
	if dt == 0 || dt > 1<<62 {
		return 0
	}
	p := float64(dt-di) / float64(dt) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func rate(prev, cur uint64, seconds float64) float64 {
	if seconds <= 0 || cur < prev {
		return 0
	}
	return float64(cur-prev) / seconds
}

// Processes devuelve los procesos ordenados por CPU y luego RSS.
func (c *Collector) Processes(limit int) []Process {
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	now := time.Now()
	c.mu.Lock()
	if len(c.procTop) > 0 && now.Sub(c.procAt) < 5*time.Second {
		out := append([]Process(nil), c.procTop...)
		c.mu.Unlock()
		if len(out) > limit {
			out = out[:limit]
		}
		return out
	}
	c.mu.Unlock()
	entries := listProcesses()
	c.mu.Lock()
	if c.prevProc == nil {
		c.prevProc = map[int]procSample{}
	}
	prev := c.prevProc
	prevAt := c.prevAt
	c.mu.Unlock()

	next := make(map[int]procSample, len(entries))
	out := make([]Process, 0, len(entries))
	for _, e := range entries {
		s, err := procstat.Read(e.pid)
		if err != nil {
			continue
		}
		p := Process{PID: e.pid, Name: e.name, RSSMB: float64(s.RSSBytes) / 1048576, Threads: s.Threads}
		if old, ok := prev[e.pid]; ok && prevAt.After(time.Time{}) {
			el := now.Sub(old.at).Seconds()
			if el > 0 {
				p.CPUPct = float64(s.CPUTime-old.cpu) / float64(time.Second) / el * 100
				if p.CPUPct < 0 {
					p.CPUPct = 0
				}
			}
		}
		next[e.pid] = procSample{cpu: s.CPUTime, at: now}
		out = append(out, p)
	}
	c.mu.Lock()
	c.prevProc = next
	c.prevAt = now
	c.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUPct == out[j].CPUPct {
			return out[i].RSSMB > out[j].RSSMB
		}
		return out[i].CPUPct > out[j].CPUPct
	})
	c.mu.Lock()
	c.procAt = now
	c.procTop = append([]Process(nil), out...)
	c.mu.Unlock()
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Monitor guarda histórico corto en memoria (1 muestra/min, 24h máx).
type Monitor struct {
	mu      sync.Mutex
	col     *Collector
	history []HistoryPoint
	last    time.Time
	now     func() time.Time
}

// NewMonitor crea el monitor con colector real.
func NewMonitor() *Monitor {
	return &Monitor{col: NewCollector(), now: time.Now}
}

// Snapshot devuelve la foto y añade un punto al histórico si pasó 1 min.
func (m *Monitor) Snapshot() (Snapshot, error) {
	s, err := m.col.Snapshot()
	if err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if m.now != nil {
		now = m.now()
	}
	if m.last.IsZero() || now.Sub(m.last) >= time.Minute {
		m.history = append(m.history, pointOf(s))
		if len(m.history) > 1440 {
			m.history = m.history[len(m.history)-1440:]
		}
		m.last = now
	}
	return s, nil
}

// Processes top de procesos (cache 5s en el colector).
func (m *Monitor) Processes(limit int) []Process {
	m.mu.Lock()
	col := m.col
	m.mu.Unlock()
	if col == nil {
		return nil
	}
	return col.Processes(limit)
}

// History devuelve los últimos puntos dentro de la ventana.
func (m *Monitor) History(hours int) []HistoryPoint {
	if hours < 1 {
		hours = 6
	}
	if hours > 24 {
		hours = 24
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cut := time.Now().Add(-time.Duration(hours) * time.Hour)
	out := []HistoryPoint{}
	for _, p := range m.history {
		t, err := time.Parse(time.RFC3339, p.Ts)
		if err != nil || t.Before(cut) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func pointOf(s Snapshot) HistoryPoint {
	maxTemp := 0.0
	for _, t := range s.Temps {
		if t.Celsius > maxTemp {
			maxTemp = t.Celsius
		}
	}
	disk := 0.0
	for _, d := range s.Disks {
		if d.Percent > disk {
			disk = d.Percent
		}
	}
	return HistoryPoint{
		Ts: s.Ts, CPU: s.CPU.Percent, Memory: s.Memory.Percent, Disk: disk,
		RxRate: s.Network.RxRate, TxRate: s.Network.TxRate, Load1: s.Load.L1, TempMax: maxTemp,
	}
}

// SortDisks ordena por punto de montaje (determinista para API/UI).
func SortDisks(d []Disk) {
	sort.Slice(d, func(i, j int) bool { return d[i].Mount < d[j].Mount })
}
