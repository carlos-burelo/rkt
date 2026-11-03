// Package health: /healthz con estado propio + alcance real de
// cada backend (dial TCP corto). Para monitoreo externo.
package health

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
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
	// HTTPOK respuesta real del puerto (false = proceso vivo pero mudo)
	HTTPOK bool `json:"http_ok"`
}

// Deps cablea healthz: config viva, versión, estado apps y restart.
// Apps/Restart nil = sin supervisor (proxy puro). Actor de deploy/
// rollback por loopback siempre "local" (sin usuario).
type Deps struct {
	Live     *config.Live
	Version  string
	Apps     func() []AppStatus
	Restart  func(name string) bool
	Deploy   func(name, actor string) (string, error)
	Rollback func(name, actor string) error
	Retry    func(id int64, actor string) (string, error)
	Build    func(name, actor string) (int64, error)
}

// NewHandler responde GET /healthz, POST /restart/<name>,
// POST /deploy/<name>, /rollback/<name>, /retry/<id> y /build/<name>;
// resto 404/405. Solo para listener loopback: sin auth.
func NewHandler(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/restart/") {
			restart(w, r, d.Restart)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/build/") {
			buildHandler(w, r, d.Build)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/deploy/") {
			deployHandler(w, r, d.Deploy)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/rollback/") {
			rollbackHandler(w, r, d.Rollback)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/retry/") {
			retryHandler(w, r, d.Retry)
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
			c := backendCheck{Target: t.String(), OK: BackendOK(t.Host)}
			if !c.OK {
				st.Status = "degraded"
			}
			st.Backends = append(st.Backends, c)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	})
}

// BackendOK dial TCP corto a host:puerto (único hogar del alcance).
func BackendOK(hostport string) bool {
	conn, err := net.DialTimeout("tcp", hostport, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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

// buildHandler compila un directorio local (POST /build/<name>).
func buildHandler(w http.ResponseWriter, r *http.Request, fn func(name, actor string) (int64, error)) {
	if r.Method != http.MethodPost {
		http.Error(w, "solo POST", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/build/")
	if name == "" || strings.Contains(name, "/") || fn == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	id, err := fn(name, "local")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"deployment": id})
}

// deployHandler ejecuta POST /deploy/<name> en el servicio (tarda minutos).
func deployHandler(w http.ResponseWriter, r *http.Request, fn func(name, actor string) (string, error)) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "solo POST", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/deploy/")
	if name == "" || strings.Contains(name, "/") || fn == nil {
		http.NotFound(w, r)
		return
	}
	rev, err := fn(name, "local")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"deployed": name, "rev": rev})
}

// rollbackHandler ejecuta POST /rollback/<name> (versión previa).
func rollbackHandler(w http.ResponseWriter, r *http.Request, fn func(name, actor string) error) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "solo POST", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/rollback/")
	if name == "" || strings.Contains(name, "/") || fn == nil {
		http.NotFound(w, r)
		return
	}
	if err := fn(name, "local"); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"rolledback": name})
}

// retryHandler ejecuta POST /retry/<id> (mismo rev).
func retryHandler(w http.ResponseWriter, r *http.Request, fn func(id int64, actor string) (string, error)) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "solo POST", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/retry/")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 || strings.Contains(raw, "/") || fn == nil {
		http.NotFound(w, r)
		return
	}
	rev, err := fn(id, "local")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"retried": rev})
}
