package logx

import (
	"os"
	"path/filepath"
	"sort"
)

// ClearLogs vacía los logs operativos (rkt.log, access.log,
// apps/*.log, deploys/*.log) y borra sus backups .1. Truncar (no borrar)
// respeta los handles abiertos del servicio (O_APPEND sigue al nuevo EOF).
// Devuelve bytes liberados por archivo. Hogar único: CLI `logs clear`,
// API y MCP la usan; `reset` no la llama (auditoría).
func ClearLogs(cfgPath string) (map[string]int64, error) {
	return ClearLogsDir(DirFor(cfgPath))
}

// ClearLogsDir igual que ClearLogs pero sobre el dir de logs directo
// (la API solo conoce LogsDir, no el cfgPath).
func ClearLogsDir(logs string) (map[string]int64, error) {
	targets := []string{
		filepath.Join(logs, "rkt.log"),
		filepath.Join(logs, "access.log"),
	}
	for _, sub := range []string{"apps", "deploys"} {
		entries, err := os.ReadDir(filepath.Join(logs, sub))
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			targets = append(targets, filepath.Join(logs, sub, n))
		}
	}
	cleared := map[string]int64{}
	for _, p := range targets {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		if err := os.Truncate(p, 0); err != nil {
			return cleared, err
		}
		cleared[p] = st.Size()
		// El backup .1 también es historia vieja: fuera.
		if b1, err := os.Stat(p + ".1"); err == nil && !b1.IsDir() {
			if err := os.Remove(p + ".1"); err != nil {
				return cleared, err
			}
			cleared[p+".1"] = b1.Size()
		}
	}
	return cleared, nil
}
