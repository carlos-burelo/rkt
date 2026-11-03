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
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/weblog"
)

// Deps dependencias inyectables. LogsDir = logx.DirFor(cfgPath).
// Restart/Deploy/Rollback los provee el servicio (nil = deshabilitado).
// Deploy/Rollback reciben actor (usuario de sesión).
type Deps struct {
	DB       *store.DB
	Live     *config.Live
	Version  string
	LogsDir  string
	Apps     func() []health.AppStatus
	Restart  func(name string) bool
	Deploy   func(name, actor string) (string, error)
	Rollback func(name, actor string) error
	Retry    func(id int64, actor string) (string, error)
	// TLS interroga cache de certs (nil = sin TLS).
	TLS func(host string) (managed bool, expires string)
}

// cloneFleet copia editable de la flota viva (Live se lee sin lock;
// mutar el puntero vivo = race con handlers).
func cloneFleet(live *config.Live) *config.Config {
	src := live.Get()
	out := *src
	out.Apps = make([]config.App, len(src.Apps))
	for i, a := range src.Apps {
		out.Apps[i] = a
		if a.Env != nil {
			env := make(map[string]string, len(a.Env))
			for k, v := range a.Env {
				env[k] = v
			}
			out.Apps[i].Env = env
		}
		if a.Deploy != nil {
			d := *a.Deploy
			d.Build = append([]string(nil), a.Deploy.Build...)
			out.Apps[i].Deploy = &d
		}
		out.Apps[i].Args = append([]string(nil), a.Args...)
	}
	out.Routes = make([]config.Route, len(src.Routes))
	for i, r := range src.Routes {
		out.Routes[i] = r
		out.Routes[i].Hosts = append([]string(nil), r.Hosts...)
	}
	return &out
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

// memo caché TTL para agregados costosos (weblog). Clave = ruta+query.
type memo struct {
	mu sync.Mutex
	m  map[string]memoVal
}

type memoVal struct {
	exp  time.Time
	body []byte
}

func newMemo() *memo { return &memo{m: map[string]memoVal{}} }

func (c *memo) get(key string, ttl time.Duration, fn func() (any, error)) ([]byte, error) {
	c.mu.Lock()
	if v, ok := c.m[key]; ok && time.Now().Before(v.exp) {
		b := v.body
		c.mu.Unlock()
		return b, nil
	}
	c.mu.Unlock()
	v, err := fn()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.m[key] = memoVal{exp: time.Now().Add(ttl), body: b}
	if len(c.m) > 32 {
		for k, x := range c.m {
			if time.Now().After(x.exp) {
				delete(c.m, k)
			}
		}
	}
	c.mu.Unlock()
	return b, nil
}

// entries lee access.log filtrado por ventana (cero = todo).
func entries(logsDir string, since time.Time, host string) []weblog.Entry {
	all := weblog.ParseFile(filepath.Join(logsDir, "access.log"))
	if since.IsZero() && host == "" {
		return all
	}
	out := all[:0]
	for _, e := range all {
		if !since.IsZero() && e.Time.Before(since) {
			continue
		}
		if host != "" && e.Host != host {
			continue
		}
		out = append(out, e)
	}
	return out
}

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
	cache := newMemo()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		n, err := d.DB.CountUsers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"setup_required": n == 0})
	})
	mux.HandleFunc("/api/v1/auth/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "solo POST")
			return
		}
		n, err := d.DB.CountUsers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if n > 0 {
			writeErr(w, http.StatusForbidden, "setup ya completado")
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
		// re-chequea bajo carrera: el primero gana, el resto 403
		if err := d.DB.CreateUser(in.User, in.Password, store.RoleAdmin); err != nil {
			th.fail(ip)
			writeErr(w, http.StatusForbidden, "setup ya completado o datos inválidos")
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
		writeJSON(w, map[string]any{"user": in.User, "role": store.RoleAdmin, "token": tok})
	})
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
	// Apps: lista/detalle sin env (los secretos solo vía /env, operator).
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
	mux.HandleFunc("/api/v1/apps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			live := d.Live.Get()
			out := []map[string]any{}
			for _, x := range live.Apps {
				out = append(out, sanitizeApp(x, live.Routes))
			}
			writeJSON(w, out)
			return
		}
		if r.Method == http.MethodPost {
			if !requireOperator(w, r, d.DB) {
				return
			}
			var in struct {
				Repo    string `json:"repo"`
				Name    string `json:"name"`
				Domain  string `json:"domain"`
				Port    int    `json:"port"`
				Command string `json:"command"`
				Dir     string `json:"dir"`
				Preset  string `json:"preset"`
				Auto    bool   `json:"auto"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			name, err := createAppFromRepo(d.DB, d.Live, in.Repo, in.Name, in.Domain, in.Port, in.Command, in.Dir, in.Preset, in.Auto)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"app": name})
			return
		}
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	})
	mux.HandleFunc("/api/v1/apps/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
		name, action := rest, ""
		if i := strings.Index(rest, "/"); i >= 0 {
			name, action = rest[:i], rest[i+1:]
		}
		a := cfgfile.FindApp(d.Live.Get(), name)
		if a == nil {
			writeErr(w, http.StatusNotFound, "app inexistente")
			return
		}
		switch {
		case action == "" && r.Method == http.MethodGet:
			writeJSON(w, sanitizeApp(*a, d.Live.Get().Routes))
		case action == "" && r.Method == http.MethodPut:
			if !requireOperator(w, r, d.DB) {
				return
			}
			var in struct {
				Command      *string   `json:"command"`
				Args         *[]string `json:"args"`
				Dir          *string   `json:"dir"`
				RestartDelay *string   `json:"restart_delay"`
				MinUptime    *string   `json:"min_uptime"`
				MemoryMB     *uint64   `json:"memory_mb"`
				Deploy       *struct {
					Repo  string   `json:"repo"`
					Ref   string   `json:"ref"`
					Build []string `json:"build"`
					Auto  bool     `json:"auto"`
				} `json:"deploy"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			cp := cloneFleet(d.Live)
			t := cfgfile.FindApp(cp, name)
			if in.Command != nil {
				t.Command = *in.Command
			}
			if in.Args != nil {
				t.Args = *in.Args
			}
			if in.Dir != nil {
				t.Dir = *in.Dir
			}
			if in.RestartDelay != nil {
				t.RestartDelay = *in.RestartDelay
			}
			if in.MinUptime != nil {
				t.MinUptime = *in.MinUptime
			}
			if in.MemoryMB != nil {
				t.MemoryMB = *in.MemoryMB
			}
			if in.Deploy != nil {
				t.Deploy = &config.Deploy{
					Repo: in.Deploy.Repo, Ref: in.Deploy.Ref,
					Build: in.Deploy.Build, Auto: in.Deploy.Auto,
				}
			}
			if err := t.ValidateApp(0); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp)
			writeJSON(w, map[string]string{"ok": "true"})
		case action == "" && r.Method == http.MethodDelete:
			if !requireOperator(w, r, d.DB) {
				return
			}
			cp := cloneFleet(d.Live)
			kept := []config.App{}
			for _, x := range cp.Apps {
				if x.Name != name {
					kept = append(kept, x)
				}
			}
			cp.Apps = kept
			if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp)
			writeJSON(w, map[string]string{"ok": "true", "hint": "revisa huérfanas: routes list"})
		case action == "restart" && r.Method == http.MethodPost:
			if !requireOperator(w, r, d.DB) {
				return
			}
			if d.Restart == nil || !d.Restart(name) {
				writeErr(w, http.StatusNotFound, "restart fallido")
				return
			}
			writeJSON(w, map[string]string{"ok": "true"})
		case action == "deploy" && r.Method == http.MethodPost:
			if !requireOperator(w, r, d.DB) {
				return
			}
			if d.Deploy == nil {
				writeErr(w, http.StatusServiceUnavailable, "deploy deshabilitado")
				return
			}
			rev, err := d.Deploy(name, mustUser(r))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, map[string]string{"rev": rev})
		case action == "rollback" && r.Method == http.MethodPost:
			if !requireOperator(w, r, d.DB) {
				return
			}
			if d.Rollback == nil {
				writeErr(w, http.StatusServiceUnavailable, "rollback deshabilitado")
				return
			}
			if err := d.Rollback(name, mustUser(r)); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, map[string]string{"ok": "true"})
		case action == "env" && r.Method == http.MethodGet:
			if !requireOperator(w, r, d.DB) {
				return
			}
			env := a.Env
			if env == nil {
				env = map[string]string{}
			}
			writeJSON(w, env)
		case action == "env" && r.Method == http.MethodPut:
			if !requireOperator(w, r, d.DB) {
				return
			}
			var in struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			if strings.TrimSpace(in.Key) == "" {
				writeErr(w, http.StatusBadRequest, "key vacía")
				return
			}
			cp := cloneFleet(d.Live)
			if err := cfgfile.SetEnv(d.DB, cp, name, strings.TrimSpace(in.Key), in.Value); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp) // inmediato (watchMerged lo reconfirma)
			writeJSON(w, map[string]string{"ok": "true"})
		case action == "env" && r.Method == http.MethodDelete:
			if !requireOperator(w, r, d.DB) {
				return
			}
			key := r.URL.Query().Get("key")
			if key == "" {
				writeErr(w, http.StatusBadRequest, "falta ?key=")
				return
			}
			cp := cloneFleet(d.Live)
			t := cfgfile.FindApp(cp, name)
			delete(t.Env, key)
			if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp) // inmediato (watchMerged lo reconfirma)
			writeJSON(w, map[string]string{"ok": "true"})
		default:
			writeErr(w, http.StatusNotFound, "ruta inválida")
		}
	})

	// Routes: misma semántica que CLI (rm quita host).
	mux.HandleFunc("/api/v1/routes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			type routeOut struct {
				Hosts  []string `json:"hosts"`
				Target string   `json:"target"`
			}
			out := []routeOut{}
			for _, x := range d.Live.Get().Routes {
				out = append(out, routeOut{x.Hosts, x.Target})
			}
			writeJSON(w, out)
		case http.MethodPost:
			if !requireOperator(w, r, d.DB) {
				return
			}
			var in struct {
				Hosts  []string `json:"hosts"`
				Target string   `json:"target"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			if len(in.Hosts) == 0 || in.Target == "" {
				writeErr(w, http.StatusBadRequest, "hosts y target requeridos")
				return
			}
			cp := cloneFleet(d.Live)
			cp.Routes = append(cp.Routes, config.Route{Hosts: in.Hosts, Target: in.Target})
			if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp) // hot-reload inmediato
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]string{"ok": "true"})
		case http.MethodDelete:
			if !requireOperator(w, r, d.DB) {
				return
			}
			host := r.URL.Query().Get("host")
			if host == "" {
				writeErr(w, http.StatusBadRequest, "falta ?host=")
				return
			}
			cp := cloneFleet(d.Live)
			kept := []config.Route{}
			removed := false
			for _, x := range cp.Routes {
				hosts := []string{}
				for _, h := range x.Hosts {
					if strings.EqualFold(h, host) {
						removed = true
						continue
					}
					hosts = append(hosts, h)
				}
				if len(hosts) > 0 {
					x.Hosts = hosts
					kept = append(kept, x)
				} else if len(x.Hosts) > 0 {
					removed = true
				}
			}
			if !removed {
				writeErr(w, http.StatusNotFound, "host no encontrado")
				return
			}
			cp.Routes = kept
			if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp) // hot-reload inmediato
			writeJSON(w, map[string]string{"ok": "true"})
		case http.MethodPut:
			if !requireOperator(w, r, d.DB) {
				return
			}
			var pin struct {
				Host   string `json:"host"`
				Target string `json:"target"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&pin); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			if pin.Host == "" || pin.Target == "" {
				writeErr(w, http.StatusBadRequest, "host y target requeridos")
				return
			}
			cp := cloneFleet(d.Live)
			found := false
			for i := range cp.Routes {
				for _, h := range cp.Routes[i].Hosts {
					if strings.EqualFold(h, pin.Host) {
						cp.Routes[i].Target = pin.Target
						found = true
					}
				}
			}
			if !found {
				writeErr(w, http.StatusNotFound, "host no encontrado")
				return
			}
			if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp) // hot-reload inmediato
			writeJSON(w, map[string]string{"ok": "true"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, "GET|POST|PUT|DELETE")
		}
	})

	// Settings: lectura viewer, escritura operator (solo conocidas).
	mux.HandleFunc("/api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			out := map[string]string{}
			for _, k := range store.KnownSettings() {
				out[k] = d.DB.GetSetting(k)
			}
			writeJSON(w, out)
		case http.MethodPut:
			if !requireOperator(w, r, d.DB) {
				return
			}
			var in struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			known := false
			for _, k := range store.KnownSettings() {
				if k == in.Key {
					known = true
				}
			}
			if !known {
				writeErr(w, http.StatusBadRequest, "setting desconocido")
				return
			}
			if err := d.DB.SetSetting(in.Key, in.Value); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, map[string]string{"ok": "true"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, "GET|PUT")
		}
	})

	// Usage 30d (rollup access.log, caché 5min). Sin cobro: solo uso.
	mux.HandleFunc("/api/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		days := queryInt(r, "days", 30)
		b, err := cache.get("usage:"+strconv.Itoa(days), 5*time.Minute, func() (any, error) {
			es := entries(d.LogsDir, time.Now().AddDate(0, 0, -days), "")
			s := weblog.Summarize(es)
			errs := s.ByClass["5xx"]
			rate := 0.0
			if s.Total > 0 {
				rate = float64(errs) / float64(s.Total)
			}
			return map[string]any{
				"days": days, "total": s.Total, "bytes": s.Bytes,
				"err_rate": rate, "p95": s.P95, "by_day": s.ByDay,
			}, nil
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})

	// Analytics por host (o global): requests, bytes, errores, p50/p95,
	// top rutas/referers, serie diaria. Sin visitors/bounce inventados.
	mux.HandleFunc("/api/v1/analytics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		host := r.URL.Query().Get("host")
		days := queryInt(r, "days", 7)
		key := "analytics:" + host + ":" + strconv.Itoa(days)
		b, err := cache.get(key, 5*time.Minute, func() (any, error) {
			es := entries(d.LogsDir, time.Now().AddDate(0, 0, -days), host)
			return weblog.Summarize(es), nil
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})

	// Observability flota: serie req/errores por hora + restarts + caídas.
	mux.HandleFunc("/api/v1/observability", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		hours := queryInt(r, "hours", 6)
		if hours > 168 {
			hours = 168
		}
		b, err := cache.get("obs:"+strconv.Itoa(hours), time.Minute, func() (any, error) {
			since := time.Now().Add(-time.Duration(hours) * time.Hour)
			es := entries(d.LogsDir, since, "")
			type bucket struct {
				Ts  string `json:"ts"`
				Req int    `json:"req"`
				Err int    `json:"err"`
			}
			buckets := make([]bucket, hours)
			for i := range buckets {
				buckets[i].Ts = since.Add(time.Duration(i+1) * time.Hour).Format("15:04")
			}
			for _, e := range es {
				i := int(e.Time.Sub(since).Hours())
				if i < 0 || i >= hours {
					continue
				}
				buckets[i].Req++
				if e.Status >= 500 {
					buckets[i].Err++
				}
			}
			restarts := 0
			if evs, err := d.DB.ListEvents(500, "", ""); err == nil {
				for _, e := range evs {
					t, err := time.Parse(time.RFC3339, e.Ts)
					if err != nil || t.Before(since) {
						continue
					}
					if e.Kind == "died" || e.Kind == "restarted" || e.Kind == "failed" {
						restarts++
					}
				}
			}
			down := []string{}
			if d.Apps != nil {
				for _, a := range d.Apps() {
					if !a.Running || a.Failed {
						down = append(down, a.Name)
					}
				}
			}
			return map[string]any{
				"hours": hours, "buckets": buckets,
				"restarts": restarts, "down": down,
			}, nil
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})

	// TLS de un dominio (desde cache ACME; sin manager = no manejado).
	mux.HandleFunc("/api/v1/domains/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/domains/")
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[1] != "tls" || parts[0] == "" {
			writeErr(w, http.StatusNotFound, "ruta inválida (…/HOST/tls)")
			return
		}
		managed, expires := false, ""
		if d.TLS != nil {
			managed, expires = d.TLS(parts[0])
		}
		writeJSON(w, map[string]any{"host": parts[0], "managed": managed, "expires": expires})
	})

	// Alertas: lista viewer, alta/baja operator. Evaluación en tick
	// del servicio (kind=alert en events).
	mux.HandleFunc("/api/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rules, err := d.DB.ListRules()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, rules)
		case http.MethodPost:
			if !requireOperator(w, r, d.DB) {
				return
			}
			var in struct {
				App       string  `json:"app"`
				Kind      string  `json:"kind"`
				Threshold float64 `json:"threshold"`
				WindowMin int     `json:"window_min"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			id, err := d.DB.AddRule(in.App, in.Kind, in.Threshold, in.WindowMin)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]int64{"id": id})
		default:
			writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
		}
	})
	mux.HandleFunc("/api/v1/alerts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
			return
		}
		if !requireOperator(w, r, d.DB) {
			return
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v1/alerts/"), 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		if err := d.DB.DeleteRule(id); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	})

	// Proxy: routes con alcance real (dial) + TLS por host.
	// Lo que Vercel esconde, rkt lo muestra: cada host, a dónde va,
	// si el backend responde y si el cert existe.
	mux.HandleFunc("/api/v1/proxy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		type routeStatus struct {
			Hosts  []string `json:"hosts"`
			Target string   `json:"target"`
			OK     bool     `json:"ok"`
			TLS    []string `json:"tls"`
		}
		out := []routeStatus{}
		for _, x := range d.Live.Get().Routes {
			rs := routeStatus{Hosts: x.Hosts, Target: x.Target, TLS: []string{}}
			if u, err := url.Parse(x.Target); err == nil && u.Host != "" {
				rs.OK = health.BackendOK(u.Host)
			}
			for _, h := range x.Hosts {
				state := "off"
				if d.TLS != nil {
					if ok, exp := d.TLS(h); ok {
						state = exp
						if state == "" {
							state = "auto"
						}
					}
				}
				rs.TLS = append(rs.TLS, state)
			}
			out = append(out, rs)
		}
		writeJSON(w, out)
	})

	// Build log de un deploy (para vista detalle) + retry mismo rev.
	mux.HandleFunc("/api/v1/deployments/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && parts[1] == "retry" && r.Method == http.MethodPost {
			if !requireOperator(w, r, d.DB) {
				return
			}
			id, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil || id <= 0 || d.Retry == nil {
				writeErr(w, http.StatusBadRequest, "id inválido o retry deshabilitado")
				return
			}
			rev, err := d.Retry(id, mustUser(r))
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, map[string]string{"rev": rev})
			return
		}
		if len(parts) != 2 || parts[1] != "log" || r.Method != http.MethodGet {
			writeErr(w, http.StatusNotFound, "ruta inválida (…/ID/log|…/ID/retry)")
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		_, lp, ok := d.DB.DeploymentLogPath(id)
		if !ok {
			writeErr(w, http.StatusNotFound, "sin log para ese deploy")
			return
		}
		out, err := logx.Tail(lp, queryInt(r, "n", 200))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]any{"log": out})
	})

	// todo autenticado salvo login/status/setup: envuelve mux (login y
	// setup se validan aparte por ruta para no romper el bootstrap
	// sin usuarios).
	authed := RequireAuth(d.DB, mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/login", "/api/v1/auth/status", "/api/v1/auth/setup":
			mux.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
}

// mustUser usuario autenticado (ruta ya pasó RequireAuth).
func mustUser(r *http.Request) string {
	u, _ := UserOf(r)
	return u
}

// createAppFromRepo alta con inferencia vía cfgfile.CreateApp (hogar
// único, también usado por MCP). Escribe vía AddApp y refresca Live.
func createAppFromRepo(db *store.DB, live *config.Live, repo, name, domain string, port int, command, dir, preset string, auto bool) (string, error) {
	app, domains, portS, err := cfgfile.CreateApp(db, repo, name, domain, port, command, dir, preset, auto)
	if err != nil {
		return "", err
	}
	if err := cfgfile.AddApp(db, app, domains, portS); err != nil {
		return "", err
	}
	if snap, err := cfgfile.Snapshot(db, live.Get()); err == nil {
		live.Swap(snap)
	}
	return app.Name, nil
}

// requireOperator exige rol operator+ (escribe 403 si no).
func requireOperator(w http.ResponseWriter, r *http.Request, db *store.DB) bool {
	_ = db
	_, role := UserOf(r)
	if roleLevel(role) < roleLevel(store.RoleOperator) {
		writeErr(w, http.StatusForbidden, "requiere rol operator")
		return false
	}
	return true
}

// sanitizeApp quita Env (secretos solo vía /env, operator).
// domains: hosts cuyas routes apuntan al PORT de la app (mapeo real,
// no adivinanza por nombre).
func sanitizeApp(a config.App, routes []config.Route) map[string]any {
	deploy := any(nil)
	if a.Deploy != nil {
		deploy = map[string]any{
			"repo": a.Deploy.Repo, "ref": a.Deploy.Ref,
			"build": a.Deploy.Build, "auto": a.Deploy.Auto,
		}
	}
	// domains: hosts cuyas routes apuntan al PORT de la app (mapeo real,
	// no adivinanza por nombre).
	domains := []string{}
	if port := a.Env["PORT"]; port != "" {
		want := "http://127.0.0.1:" + port
		for _, r := range routes {
			if r.Target == want {
				domains = append(domains, r.Hosts...)
			}
		}
	}
	return map[string]any{
		"name": a.Name, "command": a.Command, "args": a.Args,
		"dir": a.Dir, "restart_delay": a.RestartDelay,
		"min_uptime": a.MinUptime, "memory_mb": a.MemoryMB,
		"deploy": deploy, "domains": domains,
	}
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
