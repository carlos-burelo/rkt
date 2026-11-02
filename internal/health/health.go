// Package health: /healthz con estado propio + alcance real de
// cada backend (dial TCP corto). Para monitoreo externo.
package health

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"rkt-proxy/internal/config"
)

type backendCheck struct {
	Target string `json:"target"`
	OK     bool   `json:"ok"`
}

type status struct {
	Status   string         `json:"status"`
	Version  string         `json:"version"`
	Routes   int            `json:"routes"`
	Backends []backendCheck `json:"backends"`
}

// NewHandler responde en path exacto /healthz; resto 404.
func NewHandler(cfg *config.Config, version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		st := status{Status: "ok", Version: version, Routes: len(cfg.Routes)}
		seen := map[string]bool{}
		for i := range cfg.Routes {
			t := cfg.Routes[i].TargetURL()
			if t == nil || seen[t.Host] {
				continue
			}
			seen[t.Host] = true
			c := backendCheck{Target: t.String()}
			conn, err := net.DialTimeout("tcp", t.Host, 2*time.Second)
			if err == nil {
				c.OK = true
				conn.Close()
			} else {
				st.Status = "degraded"
			}
			st.Backends = append(st.Backends, c)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	})
}
