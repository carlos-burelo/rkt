// Package api: JSON versionado para dashboard/CLI/MCP.
// Contratos estables (/api/v1/*); misma capa de servicio para todos
// los clientes. Auth por credenciales + sesiones bearer desde día 1
// (fail-closed: sin usuarios, nada opera salvo crear el primero por CLI).
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	// th limita intentos de login/setup por IP. Lo inicializa NewHandler.
	th *loginThrottle
	// mu serializa mutaciones de flota: clone→mutar→guardar→swap de dos
	// requests concurrentes no puede pisarse (lost update).
	mu *sync.Mutex
}

// mutateFleet ejecuta una mutación de apps/rutas bajo lock y persiste con
// un único camino: clonar, mutar, validar+guardar, publicar en Live.
func (d *Deps) mutateFleet(apply bool, fn func(*config.Config) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp, err := cfgfile.MutateFleet(d.DB, fn)
	if err != nil {
		return err
	}
	d.Live.Swap(cp)
	if apply && d.Apply != nil {
		d.Apply(cp.Apps, cp.GlobalEnv)
	}
	return nil
}

// mutateInfra igual que mutateFleet pero persiste infra (env global,
// settings estructurales) en la tabla infra.
func (d *Deps) mutateInfra(apply bool, fn func(*config.Config) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp, err := cfgfile.MutateInfra(d.DB, fn)
	if err != nil {
		return err
	}
	d.Live.Swap(cp)
	if apply && d.Apply != nil {
		d.Apply(cp.Apps, cp.GlobalEnv)
	}
	return nil
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

// NewHandler mux /api/v1/*. Login público; resto con sesión;
// /users solo admin.
func NewHandler(d Deps) http.Handler {
	d.cache = newMemo()
	d.th = &loginThrottle{}
	d.mu = &sync.Mutex{}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/status", d.handleAuthStatus)
	mux.HandleFunc("/api/v1/auth/setup", d.handleSetup)
	mux.HandleFunc("/api/v1/auth/login", d.handleLogin)
	mux.HandleFunc("/api/v1/auth/logout", d.handleLogout)
	mux.HandleFunc("/api/v1/auth/me", d.handleMe)
	mux.HandleFunc("/api/v1/users", d.handleUsers)
	mux.HandleFunc("/api/v1/users/", d.handleUser)

	mux.HandleFunc("/api/v1/status", d.handleStatus)
	mux.HandleFunc("/api/v1/deployments", d.handleDeployments)
	mux.HandleFunc("/api/v1/events", d.handleEvents)
	// Apps: lista/detalle sin env (los secretos solo vía /env, operator).
	mux.HandleFunc("/api/v1/logs", d.handleLogs)
	// Mismo log, pero en vivo (SSE): init + chunks mientras el cliente escuche.
	mux.HandleFunc("/api/v1/logs/stream", d.handleLogStream)
	mux.HandleFunc("/api/v1/apps", d.handleApps)
	mux.HandleFunc("/api/v1/apps/", d.handleAppDetail)

	// Env global: variables compartidas por todas las apps (la app gana).
	mux.HandleFunc("/api/v1/env/global", d.handleGlobalEnv)

	// GitHub App: estado, alcance de repos e instalación.
	mux.HandleFunc("/api/v1/github", d.handleGithub)

	// Repos accesibles a la App (selector de "nueva app").
	mux.HandleFunc("/api/v1/github/repos", d.handleGithubRepos)

	// Routes: misma semántica que CLI (rm quita host).
	mux.HandleFunc("/api/v1/routes", d.handleRoutes)

	// Settings: lectura viewer, escritura operator (solo conocidas).
	mux.HandleFunc("/api/v1/settings", d.handleSettings)

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
	mux.HandleFunc("/api/v1/domains/", d.handleDomainTLS)

	// Alertas: lista viewer, alta/baja operator. Evaluación en tick
	// del servicio (kind=alert en events).
	mux.HandleFunc("/api/v1/alerts", d.handleAlerts)
	mux.HandleFunc("/api/v1/alerts/", d.handleAlert)

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

func (d *Deps) githubManager() *github.Manager {
	if d.GithubManager == nil {
		return nil
	}
	return d.GithubManager()
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
