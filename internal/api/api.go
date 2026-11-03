// Package api: JSON versionado para dashboard/CLI/MCP.
// Contratos estables (/api/v1/*); misma capa de servicio para todos
// los clientes. Auth por credenciales + sesiones bearer desde día 1
// (fail-closed: sin usuarios, nada opera salvo crear el primero por CLI).
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

// ctxUser/ctxRole usuario autenticado en el request.
type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxRole
)

// UserOf devuelve (usuario, rol) del request autenticado.
func UserOf(r *http.Request) (string, string) {
	u, _ := r.Context().Value(ctxUser).(string)
	role, _ := r.Context().Value(ctxRole).(string)
	return u, role
}

func roleLevel(r string) int {
	switch r {
	case store.RoleAdmin:
		return 3
	case store.RoleOperator:
		return 2
	case store.RoleViewer:
		return 1
	}
	return 0
}

func bearerOf(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie("rkt_token"); err == nil {
		return c.Value
	}
	return ""
}

// RequireAuth cualquier sesión válida. Inyecta usuario/rol en contexto.
func RequireAuth(db *store.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := bearerOf(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "falta credencial (login)")
			return
		}
		user, role, ok := db.SessionUser(tok)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "sesión inválida o expirada")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser, user)
		ctx = context.WithValue(ctx, ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole rol mínimo (viewer<operator<admin).
func RequireRole(db *store.DB, minRole string, next http.Handler) http.Handler {
	return RequireAuth(db, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, role := UserOf(r)
		if roleLevel(role) < roleLevel(minRole) {
			writeErr(w, http.StatusForbidden, "requiere rol "+minRole)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// loginThrottle frena fuerza bruta por IP (5 fallos -> 60s).
type loginThrottle struct {
	mu   sync.Mutex
	hits map[string]*attempt
}

type attempt struct {
	count int
	until time.Time
}

func (t *loginThrottle) blocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.hits[ip]
	if !ok {
		return false
	}
	if time.Now().After(a.until) && a.count < 5 {
		return false
	}
	return time.Now().Before(a.until)
}

func (t *loginThrottle) fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hits == nil {
		t.hits = map[string]*attempt{}
	}
	a := t.hits[ip]
	if a == nil {
		a = &attempt{}
		t.hits[ip] = a
	}
	a.count++
	if a.count >= 5 {
		a.until = time.Now().Add(time.Minute)
	}
	// techo anti-memoria: purga simple
	if len(t.hits) > 1000 {
		for k, v := range t.hits {
			if time.Now().After(v.until) && v.count < 5 {
				delete(t.hits, k)
			}
		}
	}
}

func (t *loginThrottle) ok(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.hits, ip)
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// NewHandler mux /api/v1/*. Login público; resto con sesión;
// /users solo admin.
func NewHandler(d Deps) http.Handler {
	th := &loginThrottle{}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "solo POST")
			return
		}
		ip := clientIP(r)
		if th.blocked(ip) {
			writeErr(w, http.StatusTooManyRequests, "demasiados intentos, espera 60s")
			return
		}
		var in struct {
			User     string `json:"user"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		role, err := d.DB.CheckPassword(in.User, in.Password)
		if err != nil {
			th.fail(ip)
			writeErr(w, http.StatusUnauthorized, "credenciales inválidas")
			return
		}
		th.ok(ip)
		tok, err := d.DB.NewSession(in.User, store.SessionTTL)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: "rkt_token", Value: tok, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteLaxMode,
			MaxAge: int((store.SessionTTL).Seconds()),
		})
		writeJSON(w, map[string]any{"user": in.User, "role": role, "token": tok})
	})
	mux.Handle("/api/v1/auth/logout", RequireAuth(d.DB, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := bearerOf(r); tok != "" {
			d.DB.DeleteSession(tok)
		}
		http.SetCookie(w, &http.Cookie{Name: "rkt_token", Value: "", Path: "/", MaxAge: -1})
		writeJSON(w, map[string]string{"ok": "true"})
	})))
	mux.Handle("/api/v1/auth/me", RequireAuth(d.DB, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, role := UserOf(r)
		writeJSON(w, map[string]string{"user": u, "role": role})
	})))
	mux.Handle("/api/v1/users", RequireRole(d.DB, store.RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			users, err := d.DB.ListUsers()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, users)
		case http.MethodPost:
			var in struct {
				Name     string `json:"name"`
				Password string `json:"password"`
				Role     string `json:"role"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			if err := d.DB.CreateUser(in.Name, in.Password, in.Role); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"ok": "true"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
		}
	})))
	mux.Handle("/api/v1/users/", RequireRole(d.DB, store.RoleAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
		if name == "" || strings.Contains(name, "/") {
			writeErr(w, http.StatusBadRequest, "nombre inválido")
			return
		}
		if err := d.DB.DeleteUser(name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	})))

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

	// todo autenticado salvo login: envuelve mux (login se valida aparte
	// por ruta para no romper el bootstrap sin usuarios).
	authed := RequireAuth(d.DB, mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			mux.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
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
