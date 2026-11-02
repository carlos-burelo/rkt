// Package api: JSON versionado para dashboard/CLI/MCP.
// Contratos estables (/api/v1/*); misma capa de servicio para todos
// los clientes. Auth por token desde día 1 (fail-closed: sin token
// configurado, todo 401). Sin cablear aún (siguiente átomo).
package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

// Deps dependencias inyectables. LogsDir = logx.DirFor(cfgPath).
type Deps struct {
	DB      *store.DB
	Live    *config.Live
	Version string
	LogsDir string
	Apps    func() []health.AppStatus
}

// NewHandler mux /api/v1/* con auth. Sin token válido: 401 JSON.
func NewHandler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		apps := []health.AppStatus{}
		if d.Apps != nil {
			apps = d.Apps()
		}
		writeJSON(w, map[string]any{
			"version": d.Version,
			"routes":  len(d.Live.Get().Routes),
			"apps":    apps,
		})
	})
	mux.HandleFunc("/api/v1/deployments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		n := queryInt(r, "n", 20)
		deps, err := d.DB.ListDeployments(r.URL.Query().Get("app"), n)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, deps)
	})
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		q := r.URL.Query()
		evs, err := d.DB.ListEvents(queryInt(r, "n", 20), q.Get("kind"), q.Get("app"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, evs)
	})
	mux.HandleFunc("/api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		path := filepath.Join(d.LogsDir, "rkt.log")
		if app := r.URL.Query().Get("app"); app != "" {
			if !validAppName(app) {
				writeErr(w, http.StatusBadRequest, "app inválida [a-z0-9-]")
				return
			}
			path = filepath.Join(d.LogsDir, "apps", app+".log")
		}
		out, err := logx.Tail(path, queryInt(r, "n", 50))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]any{"log": out})
	})
	return RequireToken(d.DB, mux)
}

// RequireToken Bearer o cookie rkt_token == settings.dashboard_token.
// Sin token configurado: 401 fail-closed (nada opera sin auth).
func RequireToken(db *store.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := db.GetSetting(store.KeyDashboardToken)
		if want == "" {
			writeErr(w, http.StatusUnauthorized, "sin dashboard_token (settings set dashboard_token=...)")
			return
		}
		got := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			got = strings.TrimPrefix(h, "Bearer ")
		} else if c, err := r.Cookie("rkt_token"); err == nil {
			got = c.Value
		}
		if got == "" || got != want {
			writeErr(w, http.StatusUnauthorized, "token inválido")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func queryInt(r *http.Request, key string, def int) int {
	if s := r.URL.Query().Get(key); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func validAppName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
