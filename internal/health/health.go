// Package health: /healthz con estado propio + alcance real de
// cada backend (dial TCP corto). Para monitoreo externo.
package health

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
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
	Apps     []AppStatus    `json:"apps,omitempty"`
}

// AppStatus lo provee el supervisor (apps nil = sin apps).
type AppStatus struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Pid      int    `json:"pid,omitempty"`
	Restarts int    `json:"restarts"`
	Failed   bool   `json:"failed,omitempty"`
}

// Deps cablea healthz: config viva, versión, estado apps y restart.
// Apps/Restart nil = sin supervisor (proxy puro).
type Deps struct {
	Live    *config.Live
	Version string
	Apps    func() []AppStatus
	Restart func(name string) bool
}

// NewHandler responde GET /healthz y POST /restart/<name>; resto 404/405.
// Solo para listener loopback: restart no lleva auth.
func NewHandler(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/restart/") {
			restart(w, r, d.Restart)
			return
		}
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		cfg := d.Live.Get()
		st := status{Status: "ok", Version: d.Version, Routes: len(cfg.Routes)}
		if d.Apps != nil {
			st.Apps = d.Apps()
			for _, a := range st.Apps {
				if !a.Running || a.Failed {
					st.Status = "degraded"
				}
			}
		}
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

// restart ejecuta POST /restart/<name> contra el supervisor.
func restart(w http.ResponseWriter, r *http.Request, fn func(name string) bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "solo POST", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/restart/")
	if name == "" || strings.Contains(name, "/") || fn == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !fn(name) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "app inexistente"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"restarted": name})
}
