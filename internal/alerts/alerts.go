// Package alerts: evaluador puro de reglas (testeable sin DB).
// El servicio junta datos (events + weblog) y persiste disparos.
package alerts

import (
	"strconv"
	"time"

	"rkt-proxy/internal/store"
)

// Fired evalúa una regla con conteos ya calculados en su ventana.
// Cooldown = ventana (no re-dispara hasta que pasa).
func Fired(r store.AlertRule, failed int, errRate float64, now time.Time) bool {
	if r.LastFired != "" {
		if t, err := time.Parse(time.RFC3339, r.LastFired); err == nil {
			if now.Before(t.Add(time.Duration(r.WindowMin) * time.Minute)) {
				return false
			}
		}
	}
	switch r.Kind {
	case "failed":
		return float64(failed) >= r.Threshold
	case "error_rate":
		return errRate >= r.Threshold
	}
	return false
}

// Describe mensaje del evento kind=alert.
func Describe(r store.AlertRule) string {
	if r.Kind == "failed" {
		return "fallos en ventana (regla #" + strconv.FormatInt(r.ID, 10) + ")"
	}
	return "error_rate alto (regla #" + strconv.FormatInt(r.ID, 10) + ")"
}
