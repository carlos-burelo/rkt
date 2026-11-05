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
	"strconv"
	"strings"
	"sync"
	"time"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/store"
)

// Deps dependencias inyectables. LogsDir = logx.DirFor(cfgPath).
// Restart/Deploy/Rollback los provee el servicio (nil = deshabilitado).
// Deploy/Rollback reciben actor (usuario de sesión).
type Deps struct {
	DB      *store.DB
	Live    *config.Live
	Version string
	LogsDir string
	Apps    func() []health.AppStatus
	Restart func(name string) bool
	// Apply aplica el manifiesto al supervisor (arranca/reinicia/detiene).
	// global se fusiona en el env efectivo de cada app.
	Apply  func([]config.App, map[string]string)
	Deploy func(name, actor string) (string, error)
	// DeployAsync arranca el deploy y devuelve el id para seguirlo en vivo.
	DeployAsync func(name, actor string) (int64, error)
	Rollback    func(name, actor string) error
	Retry       func(id int64, actor string) (string, error)
	// Build compila una app local con su plan y devuelve el id de la salida.
	Build func(name, actor string) (int64, error)
	// TLS interroga cache de certs (nil = sin TLS).
	TLS func(host string) (managed bool, expires string)
	// Github estado vivo de la GitHub App (nil = no configurada).
	Github func() (map[string]any, error)
	// GithubRepos repos accesibles a las instalaciones (selector de app).
	GithubRepos func() ([]github.Repo, error)
	// GithubManager manager vivo para inferir plan de repos privados.
	GithubManager func() *github.Manager

	// cache agregados costosos (access.log). Lo inicializa NewHandler.
	cache *memo
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
// Rechaza mutaciones con Origin de otro host (defensa CSRF además de SameSite).
func RequireAuth(db *store.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					writeErr(w, http.StatusForbidden, "origen no permitido")
					return
				}
			}
		}
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
	d.cache = newMemo()
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
		// re-chequea bajo carrera: el primero gana, el resto 403.
		// validación primero para error 400 exacto (no mentir).
		if err := store.ValidateCredential(in.User, in.Password); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.DB.CreateUser(in.User, in.Password, store.RoleAdmin); err != nil {
			th.fail(ip)
			writeErr(w, http.StatusForbidden, "setup ya completado")
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
	mux.HandleFunc("/api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if tok := bearerOf(r); tok != "" {
			d.DB.DeleteSession(tok)
		}
		http.SetCookie(w, &http.Cookie{Name: "rkt_token", Value: "", Path: "/", MaxAge: -1})
		writeJSON(w, map[string]string{"ok": "true"})
	})
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		u, role := UserOf(r)
		writeJSON(w, map[string]string{"user": u, "role": role})
	})
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
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
	})
	mux.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r) {
			return
		}
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
	})

	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		apps := []health.AppStatus{}
		if d.Apps != nil {
			apps = d.Apps()
		}
		cfg := d.Live.Get()
		httpPort := listenPort(cfg.ListenHTTP, "80")
		httpsPort := ""
		if cfg.ListenHTTPS != "" {
			httpsPort = listenPort(cfg.ListenHTTPS, "443")
		}
		writeJSON(w, map[string]any{
			"version":    d.Version,
			"routes":     len(cfg.Routes),
			"apps":       apps,
			"http_port":  httpPort,
			"https_port": httpsPort,
		})
	})
	mux.HandleFunc("/api/v1/deployments", d.handleDeployments)
	mux.HandleFunc("/api/v1/events", d.handleEvents)
	// Apps: lista/detalle sin env (los secretos solo vía /env, operator).
	mux.HandleFunc("/api/v1/logs", d.handleLogs)
	// Mismo log, pero en vivo (SSE): init + chunks mientras el cliente escuche.
	mux.HandleFunc("/api/v1/logs/stream", d.handleLogStream)
	mux.HandleFunc("/api/v1/apps", d.handleApps)
	mux.HandleFunc("/api/v1/apps/", d.handleAppDetail)

	// Env global: variables compartidas por todas las apps (la app gana).
	mux.HandleFunc("/api/v1/env/global", func(w http.ResponseWriter, r *http.Request) {
		if !requireOperator(w, r) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			env := d.Live.Get().GlobalEnv
			if env == nil {
				env = map[string]string{}
			}
			writeJSON(w, env)
		case http.MethodPut:
			var in struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeErr(w, http.StatusBadRequest, "JSON inválido")
				return
			}
			key := strings.TrimSpace(in.Key)
			if key == "" {
				writeErr(w, http.StatusBadRequest, "key vacía")
				return
			}
			cp := d.Live.Get().Clone()
			if err := cfgfile.SetGlobalEnv(d.DB, cp, key, in.Value); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp)
			if d.Apply != nil {
				d.Apply(cp.Apps, cp.GlobalEnv)
			}
			writeJSON(w, map[string]string{"ok": "true", "hint": "las apps heredan la global al reiniciar"})
		case http.MethodDelete:
			key := r.URL.Query().Get("key")
			if key == "" {
				writeErr(w, http.StatusBadRequest, "falta ?key=")
				return
			}
			cp := d.Live.Get().Clone()
			if err := cfgfile.UnsetGlobalEnv(d.DB, cp, key); err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			d.Live.Swap(cp)
			if d.Apply != nil {
				d.Apply(cp.Apps, cp.GlobalEnv)
			}
			writeJSON(w, map[string]string{"ok": "true"})
		default:
			writeErr(w, http.StatusMethodNotAllowed, "método no permitido")
		}
	})

	// GitHub App: estado, alcance de repos e instalación.
	mux.HandleFunc("/api/v1/github", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		if d.Github == nil {
			writeJSON(w, map[string]any{"configured": false})
			return
		}
		st, err := d.Github()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, st)
	})

	// Repos accesibles a la App (selector de "nueva app").
	mux.HandleFunc("/api/v1/github/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErr(w, http.StatusMethodNotAllowed, "solo GET")
			return
		}
		if d.GithubRepos == nil {
			writeJSON(w, []github.Repo{})
			return
		}
		repos, err := d.GithubRepos()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, repos)
	})

	// Routes: misma semántica que CLI (rm quita host).
	mux.HandleFunc("/api/v1/routes", d.handleRoutes)

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
			if !requireOperator(w, r) {
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

	// Navegador de rutas para los pickers del dashboard (operator+).
	mux.HandleFunc("/api/v1/fs", d.handleFS)

	// Detección de plan sin crear app (preview Vercel-like).
	mux.HandleFunc("/api/v1/detect", d.handleDetect)

	// Sugerencia de dominios (localhost/test/base) para el editor de rutas.
	mux.HandleFunc("/api/v1/domains/suggest", d.handleDomainsSuggest)

	// Zonas locales agrupadas (panel tipo DNS): base + subdominios.
	mux.HandleFunc("/api/v1/domains/zones", d.handleZones)

	// Hosts del sistema: arreglar DNS de *.test/base desde la UI.
	mux.HandleFunc("/api/v1/domains/hosts", d.handleHosts)

	// Usage 30d (rollup access.log, caché 5min). Sin cobro: solo uso.
	mux.HandleFunc("/api/v1/usage", d.handleUsage)

	// Analytics por host (o global), sin métricas inventadas.
	mux.HandleFunc("/api/v1/analytics", d.handleAnalytics)

	// Observability flota: serie req/errores por hora + restarts + caídas.
	mux.HandleFunc("/api/v1/observability", d.handleObservability)

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
			if !requireOperator(w, r) {
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
		if !requireOperator(w, r) {
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
	mux.HandleFunc("/api/v1/proxy", d.handleProxy)

	// Build log de un deploy (para vista detalle) + retry mismo rev.
	mux.HandleFunc("/api/v1/deployments/", d.handleDeploymentDetail)

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

func (d *Deps) githubManager() *github.Manager {
	if d.GithubManager == nil {
		return nil
	}
	return d.GithubManager()
}

// requireOperator exige rol operator+ (escribe 403 si no).
func requireOperator(w http.ResponseWriter, r *http.Request) bool {
	_, role := UserOf(r)
	if roleLevel(role) < roleLevel(store.RoleOperator) {
		writeErr(w, http.StatusForbidden, "requiere rol operator")
		return false
	}
	return true
}

// requireAdmin exige rol admin (gestión de usuarios).
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, role := UserOf(r)
	if roleLevel(role) < roleLevel(store.RoleAdmin) {
		writeErr(w, http.StatusForbidden, "requiere rol admin")
		return false
	}
	return true
}

// listenPort extrae el puerto de ":8080" / "127.0.0.1:8080".
func listenPort(addr, def string) string {
	if addr == "" {
		return def
	}
	if i := strings.LastIndex(addr, ":"); i >= 0 && i+1 < len(addr) {
		return addr[i+1:]
	}
	return def
}

func queryInt(r *http.Request, key string, def int) int {
	if s := r.URL.Query().Get(key); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return def
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
