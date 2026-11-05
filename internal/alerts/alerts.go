// Package alerts: evaluador puro de reglas (testeable sin DB).
package alerts

import (
	"strconv"
	"time"

	"rkt-proxy/internal/store"
)

// Metrics valores medidos para la ventana de una regla.
type Metrics struct {
	Failed   int
	ErrRate  float64
	P95ms    float64
	MemoryMB float64
	DownMin  int
	DiskPct  float64
	CPUPct   float64
	MemPct   float64
	TempC    float64
}

// Value métrica usada por el kind (para historial y mensajes).
func (m Metrics) Value(kind string) float64 {
	switch kind {
	case "failed":
		return float64(m.Failed)
	case "error_rate":
		return m.ErrRate
	case "latency_p95":
		return m.P95ms
	case "memory_mb":
		return m.MemoryMB
	case "down_min":
		return float64(m.DownMin)
	case "disk_pct":
		return m.DiskPct
	case "cpu_pct":
		return m.CPUPct
	case "mem_pct":
		return m.MemPct
	case "temp_c":
		return m.TempC
	}
	return 0
}

// Fired evalúa una regla con métricas ya calculadas en su ventana.
// Cooldown = ventana (no re-dispara hasta que pasa).
func Fired(r store.AlertRule, m Metrics, now time.Time) bool {
	if !r.Enabled {
		return false
	}
	if r.LastFired != "" {
		if t, err := time.Parse(time.RFC3339, r.LastFired); err == nil {
			if now.Before(t.Add(time.Duration(r.WindowMin) * time.Minute)) {
				return false
			}
		}
	}
	switch r.Kind {
	case "failed":
		return float64(m.Failed) >= r.Threshold
	case "error_rate":
		return m.ErrRate >= r.Threshold
	case "latency_p95":
		return m.P95ms >= r.Threshold
	case "memory_mb":
		return m.MemoryMB >= r.Threshold
	case "down_min":
		return float64(m.DownMin) >= r.Threshold
	case "disk_pct":
		return m.DiskPct >= r.Threshold
	case "cpu_pct":
		return m.CPUPct >= r.Threshold
	case "mem_pct":
		return m.MemPct >= r.Threshold
	case "temp_c":
		return m.TempC >= r.Threshold
	}
	return false
}

// Describe mensaje del evento kind=alert.
func Describe(r store.AlertRule, m Metrics) string {
	v := m.Value(r.Kind)
	return "regla #" + strconv.FormatInt(r.ID, 10) + " · " + r.Kind + " " +
		format(v) + " ≥ " + format(r.Threshold)
}

func format(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
