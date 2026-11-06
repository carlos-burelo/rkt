// Package config: modelo, defaults y validación de la configuración.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	pathpkg "path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/platform"
)

// Route mapea uno o varios hosts al mismo backend.
type Route struct {
	Hosts  []string `yaml:"hosts"`
	Target string   `yaml:"target"`

	targetURL *url.URL `yaml:"-"`
}

// TargetURL backend ya parseado.
func (r *Route) TargetURL() *url.URL { return r.targetURL }

// Resolve parsea el target (sin chequeo de duplicados global).
// ws/wss se normalizan a http/https para el transporte; el upgrade se
// conserva por las cabeceras de la petición.
func (r *Route) Resolve() error {
	u, err := url.Parse(r.Target)
	if err != nil || u.Host == "" {
		return fmt.Errorf("target inválido %q: debe ser http(s):// o ws(s)://127.0.0.1:puerto", r.Target)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		u.Scheme = "http"
	case "https", "wss":
		u.Scheme = "https"
	default:
		return fmt.Errorf("target inválido %q: esquema %q no soportado (http|https|ws|wss)", r.Target, u.Scheme)
	}
	r.targetURL = u
	return nil
}

// Tipos de tarea programada.
const (
	CronCommand = "command"
	CronHTTP    = "http"
)

// CronJob tarea programada de una app. `command` ejecuta en su directorio con
// el entorno efectivo; `http` llama al puerto local de la app. `Disabled`
// evita programarla. Type vacío = command (compatibilidad).
type CronJob struct {
	Name     string `yaml:"name" json:"name"`
	Schedule string `yaml:"schedule" json:"schedule"`
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Command  string `yaml:"command,omitempty" json:"command,omitempty"`
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	Method   string `yaml:"method,omitempty" json:"method,omitempty"`
	Timeout  string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Kind tipo efectivo, con command como default histórico.
func (j CronJob) Kind() string {
	if t := strings.ToLower(strings.TrimSpace(j.Type)); t != "" {
		return t
	}
	return CronCommand
}

// HTTPMethod método efectivo de una tarea http (GET por defecto).
func (j CronJob) HTTPMethod() string {
	if m := strings.ToUpper(strings.TrimSpace(j.Method)); m != "" {
		return m
	}
	return "GET"
}

// ValidCronName [a-z0-9-], igual que las apps: seguro para logs.
func ValidCronName(s string) bool { return ValidAppName(s) }

// ParseCron valida una expresión estándar de 5 campos.
func ParseCron(expr string) (cron.Schedule, error) {
	s, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, fmt.Errorf("cron inválido %q: usa minuto hora día mes semana", expr)
	}
	return s, nil
}

// CronTimeout duración máxima de una ejecución (default 10m).
func (j CronJob) CronTimeout() (time.Duration, error) {
	if strings.TrimSpace(j.Timeout) == "" {
		return 10 * time.Minute, nil
	}
	d, err := time.ParseDuration(j.Timeout)
	if err != nil || d < time.Second || d > time.Hour {
		return 0, fmt.Errorf("timeout inválido %q (1s..1h)", j.Timeout)
	}
	return d, nil
}

// App es un proceso supervisado (mini-PaaS). El proxy enruta
// a sus puertos; rkt los levanta y reinicia si mueren.
// Deploy publica la app desde git: pull+build+swap+restart+rollback.
type Deploy struct {
	Repo     string   `yaml:"repo"`
	Ref      string   `yaml:"ref"`
	TokenEnv string   `yaml:"token_env"`
	Build    []string `yaml:"build"`
	// Release pasos de migración/release; corren en staging tras el build
	// y antes del swap, con el env efectivo de la app.
	Release []string `yaml:"release,omitempty"`
	Auto    bool     `yaml:"auto"`
}

// Github auth vía GitHub App para repos privados (tokens 1h, cacheados).
// Multi-instalación (org + personal): se registran solas vía setup
// del browser o eventos de instalación. Sin installation_id fijo.
type Github struct {
	AppID         int64  `yaml:"app_id" json:"app_id"`
	AppSlug       string `yaml:"app_slug" json:"app_slug"`
	KeyFile       string `yaml:"key_file" json:"key_file"`
	PollInterval  string `yaml:"poll_interval" json:"poll_interval"`
	WebhookSecret string `yaml:"webhook_secret" json:"webhook_secret"`

	poll time.Duration `yaml:"-"`
}

// Poll intervalo de auto-deploy (parseado en Load).
func (g *Github) Poll() time.Duration { return g.poll }

// EnsureParsed aplica defaults y parsea poll (para overlay de settings).
func (g *Github) EnsureParsed() error {
	if g.PollInterval == "" {
		g.PollInterval = "10s"
	}
	p, err := time.ParseDuration(g.PollInterval)
	if err != nil || p < 10*time.Second {
		return fmt.Errorf("github poll_interval inválido %q (mín 10s)", g.PollInterval)
	}
	g.poll = p
	return nil
}

// Runtime de una app: managed (supervisor + pipeline) o external (solo
// registro de proxy; el proceso dev lo corre el humano). Sin modo global.
const (
	RuntimeManaged  = "managed"
	RuntimeExternal = "external"
)

type App struct {
	Name         string            `yaml:"name" json:"name"`
	Runtime      string            `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	Command      string            `yaml:"command" json:"command"`
	Args         []string          `yaml:"args" json:"args"`
	Dir          string            `yaml:"dir" json:"dir"`
	EnvFile      string            `yaml:"env_file" json:"env_file"`
	Env          map[string]string `yaml:"env" json:"env"`
	RestartDelay string            `yaml:"restart_delay" json:"restart_delay"`
	MinUptime    string            `yaml:"min_uptime" json:"min_uptime"`
	MemoryMB     uint64            `yaml:"memory_mb" json:"memory_mb"`
	// Disabled persiste una app detenida: el supervisor no la arranca.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	// HealthPath ruta HTTP de la sonda local (default "/").
	HealthPath string `yaml:"health_path,omitempty" json:"health_path,omitempty"`
	// Static dir servido por el propio binario (sitios: astro/vite/export).
	Static string `yaml:"static,omitempty" json:"static,omitempty"`
	// Persist rutas relativas que sobreviven al swap de deploy (data,
	// uploads, sqlite local). Se copian de live a staging antes del swap.
	Persist []string `yaml:"persist,omitempty" json:"persist,omitempty"`
	// BasicAuth "usuario:hash_bcrypt" para proteger la app en el edge.
	BasicAuth string `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
	// AllowIPs CIDRs permitidos (vacío = todos).
	AllowIPs []string `yaml:"allow_ips,omitempty" json:"allow_ips,omitempty"`
	// Maintenance responde 503 sin tocar el backend.
	Maintenance bool    `yaml:"maintenance,omitempty" json:"maintenance,omitempty"`
	Deploy      *Deploy `yaml:"deploy" json:"deploy"`
	// Cron tareas programadas de la app.
	Cron []CronJob `yaml:"cron,omitempty" json:"cron,omitempty"`

	delay time.Duration `yaml:"-" json:"-"`
	minUp time.Duration `yaml:"-" json:"-"`
}

// External true si la app solo usa el proxy; no la arranca el supervisor.
func (a *App) External() bool { return a.Runtime == RuntimeExternal }

// Delay espera base entre reinicios (parseado en Load).
func (a *App) Delay() time.Duration { return a.delay }

// MinUp vida mínima para no contar como crash (parseado en Load).
func (a *App) MinUp() time.Duration { return a.minUp }

// SetDelays fija duraciones (strings canónicos + parseado). Para snapshot DB.
func (a *App) SetDelays(delay, minUp time.Duration) {
	a.RestartDelay = delay.String()
	a.MinUptime = minUp.String()
	a.delay, a.minUp = delay, minUp
}

// TLSCert certificado propio para uno o varios hosts (sin ACME).
type TLSCert struct {
	Hosts    []string `yaml:"hosts" json:"hosts"`
	CertFile string   `yaml:"cert_file" json:"cert_file"`
	KeyFile  string   `yaml:"key_file" json:"key_file"`
}

// HealthzConfig expone salud en listener loopback dedicado
// (no compite con paths de backends ni se expone a internet).
type HealthzConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
}

// RateLimitConfig frena abuso por IP (bots) antes del backend.
type RateLimitConfig struct {
	Enabled bool     `yaml:"enabled" json:"enabled"`
	RPS     float64  `yaml:"requests_per_second" json:"requests_per_second"`
	Burst   int      `yaml:"burst" json:"burst"`
	Allow   []string `yaml:"allow" json:"allow,omitempty"`
}

// maxBodyDefaultMB tope de body por request (anti-DoS a backends).
const maxBodyDefaultMB = 10

// restartGraceDefault cubre el hueco de un restart de app.
const restartGraceDefault = "10s"

// defaultDenyPaths escaneos ubicuos; jamás legítimos en backends propios.
var defaultDenyPaths = []string{
	"/.env",
	"/.git/*",
	"/.DS_Store",
	"/php-cgi*",
	"/php-cgi/*",
	"/cgi-bin/*",
	"/info.php",
	"/wp-*",
}

// Config canónica. Un solo camino, nada legacy.
type Config struct {
	ListenHTTP   string `yaml:"listen_http" json:"listen_http"`
	ListenHTTPS  string `yaml:"listen_https" json:"listen_https"`
	ACMEEmail    string `yaml:"acme_email" json:"acme_email"`
	ACMEStaging  bool   `yaml:"acme_staging" json:"acme_staging"`
	ACMECacheDir string `yaml:"acme_cache_dir" json:"acme_cache_dir"`
	// LocalCA firma TLS con la CA propia de rkt (dev). Sin ACME.
	LocalCA bool `yaml:"local_ca" json:"local_ca,omitempty"`
	// ACMEMaxNewHosts limita emisiones nuevas por hora (-1 = sin tope).
	// Evita que subdominios random de un wildcard agoten el rate limit.
	ACMEMaxNewHosts int `yaml:"acme_max_new_hosts" json:"acme_max_new_hosts"`
	// ACMEDNSPrecheck exige que el host resuelva antes de pedir cert.
	ACMEDNSPrecheck     bool            `yaml:"acme_dns_precheck" json:"acme_dns_precheck"`
	RedirectHTTPToHTTPS bool            `yaml:"redirect_http_to_https" json:"redirect_http_to_https"`
	Healthz             HealthzConfig   `yaml:"healthz" json:"healthz"`
	RateLimit           RateLimitConfig `yaml:"rate_limit" json:"rate_limit"`
	RestartGrace        string          `yaml:"restart_grace" json:"restart_grace"`
	MaxBodyMB           int64           `yaml:"max_body_mb" json:"max_body_mb"`
	Github              *Github         `yaml:"github" json:"github,omitempty"`
	DenyPaths           []string        `yaml:"deny_paths" json:"deny_paths"`
	TLSCerts            []TLSCert       `yaml:"tls_certs" json:"tls_certs,omitempty"`
	// GlobalEnv se aplica a todas las apps; la app puede sobrescribir claves.
	GlobalEnv map[string]string `yaml:"global_env" json:"global_env,omitempty"`
	Apps      []App             `yaml:"apps" json:"-"`
	Routes    []Route           `yaml:"routes" json:"-"`

	grace time.Duration `yaml:"-"`
}

// Grace ventana de reintento ante backend caído (parseado en Load).
func (c *Config) Grace() time.Duration { return c.grace }

// MergeEnv combina globales + overrides (gana el override). Devuelve un mapa
// nuevo: el manifiesto guardado no se toca.
func MergeEnv(global, override map[string]string) map[string]string {
	if len(global) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(global)+len(override))
	for k, v := range global {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

// RuntimeEnv entorno efectivo de una app: globales pisadas por las suyas.
func (c *Config) RuntimeEnv(appEnv map[string]string) map[string]string {
	return MergeEnv(c.GlobalEnv, appEnv)
}

func defaultConfig() Config {
	return Config{
		ListenHTTP:          ":80",
		ListenHTTPS:         "",
		ACMECacheDir:        platform.DefaultCertsDir(),
		ACMEMaxNewHosts:     30,
		RedirectHTTPToHTTPS: true,
		Healthz:             HealthzConfig{Enabled: true, Listen: "127.0.0.1:8090"},
		RateLimit:           RateLimitConfig{Enabled: true, RPS: 20, Burst: 40},
		MaxBodyMB:           maxBodyDefaultMB,
		DenyPaths:           append([]string{}, defaultDenyPaths...),
	}
}

// Default config base sin apps/rutas (semilla de la DB).
func Default() *Config {
	c := defaultConfig()
	return &c
}

// Finalize aplica defaults, valida y resuelve env (config sin YAML).
func (c *Config) Finalize() error {
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	return c.ResolveEnvAll()
}

// Portable es la config completa (infra + flota + settings) para
// exportar/importar como JSON. La fuente viva sigue siendo SQLite.
type Portable struct {
	Config
	Apps     []App             `json:"apps"`
	Routes   []Route           `json:"routes"`
	Settings map[string]string `json:"settings,omitempty"`
}

// SecretMask sustituye valores de entorno en un export redactado.
// ImportJSONMerged los rellena desde la config actual; ImportJSON los rechaza.
const SecretMask = "__RKT_REDACTED__"

// ExportJSON serializa config + flota + settings con los secretos
// redactados. Para un respaldo restaurable usar ExportJSONWithSecrets.
func ExportJSON(cfg *Config, settings map[string]string) ([]byte, error) {
	return exportJSON(cfg, settings, false)
}

// ExportJSONWithSecrets incluye los valores de entorno. Solo para
// respaldos cifrados o destinos de confianza.
func ExportJSONWithSecrets(cfg *Config, settings map[string]string) ([]byte, error) {
	return exportJSON(cfg, settings, true)
}

func exportJSON(cfg *Config, settings map[string]string, includeSecrets bool) ([]byte, error) {
	cp := cfg
	if !includeSecrets {
		cp = redactConfig(cfg)
	}
	doc := Portable{Config: *cp, Apps: cp.Apps, Routes: cp.Routes, Settings: settings}
	return json.MarshalIndent(doc, "", "  ")
}

// redactConfig copia la config conservando claves y sustituyendo valores.
func redactConfig(cfg *Config) *Config {
	cp := cfg.Clone()
	if cp.GlobalEnv != nil {
		for k := range cp.GlobalEnv {
			cp.GlobalEnv[k] = SecretMask
		}
	}
	for i := range cp.Apps {
		for k := range cp.Apps[i].Env {
			cp.Apps[i].Env[k] = SecretMask
		}
	}
	return cp
}

// ImportJSON valida un documento portable y devuelve config + settings.
// Rechaza exports redactados: no se pueden restaurar sin los valores.
func ImportJSON(data []byte) (*Config, map[string]string, error) {
	return ImportJSONMerged(data, nil)
}

// ImportJSONMerged como ImportJSON, pero rellena los valores redactados
// desde prev. Es el camino para importar un export saneado sin perder
// secretos. Si falta una clave, devuelve error accionable.
func ImportJSONMerged(data []byte, prev *Config) (*Config, map[string]string, error) {
	doc := Portable{Config: *Default()}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("json inválido: %w", err)
	}
	cfg := doc.Config
	cfg.Apps = doc.Apps
	cfg.Routes = doc.Routes
	if err := cfg.Finalize(); err != nil {
		return nil, nil, err
	}
	if err := fillMasked(&cfg, prev); err != nil {
		return nil, nil, err
	}
	return &cfg, doc.Settings, nil
}

// fillMasked restaura los valores marcados con SecretMask desde prev.
func fillMasked(dst, prev *Config) error {
	var prevGlobal map[string]string
	prevApps := map[string]map[string]string{}
	if prev != nil {
		prevGlobal = prev.GlobalEnv
		for _, a := range prev.Apps {
			prevApps[a.Name] = a.Env
		}
	}
	for k, v := range dst.GlobalEnv {
		if v != SecretMask {
			continue
		}
		old, ok := prevGlobal[k]
		if !ok {
			return fmt.Errorf("global_env %s redactado y sin valor previo: usa config export --include-secrets", k)
		}
		dst.GlobalEnv[k] = old
	}
	for i := range dst.Apps {
		a := &dst.Apps[i]
		for k, v := range a.Env {
			if v != SecretMask {
				continue
			}
			old, ok := prevApps[a.Name][k]
			if !ok {
				return fmt.Errorf("apps[%s].env %s redactado y sin valor previo: usa config export --include-secrets", a.Name, k)
			}
			a.Env[k] = old
		}
	}
	return nil
}

// ApplyDefaults rellena valores omitidos. Idempotente.
func (cfg *Config) ApplyDefaults() {
	if cfg.ListenHTTP == "" {
		cfg.ListenHTTP = ":80"
	}
	// listen_https vacío = TLS desactivado (modo local). A propósito sin default.
	if cfg.ACMECacheDir == "" {
		cfg.ACMECacheDir = platform.DefaultCertsDir()
	}
	if cfg.ACMEMaxNewHosts == 0 {
		cfg.ACMEMaxNewHosts = 30
	}
	if cfg.RestartGrace == "" {
		cfg.RestartGrace = restartGraceDefault
	}
	if cfg.Github != nil && cfg.Github.PollInterval == "" {
		cfg.Github.PollInterval = "10s"
	}
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if a.Runtime == "" {
			a.Runtime = RuntimeManaged
		}
		if a.RestartDelay == "" {
			a.RestartDelay = "5s"
		}
		if a.MinUptime == "" {
			a.MinUptime = "10s"
		}
		if a.HealthPath == "" {
			a.HealthPath = "/"
		}
		if !a.External() {
			normalizeNodePM(a)
		}
		if a.Deploy != nil {
			if a.Deploy.Ref == "" {
				a.Deploy.Ref = "main"
			}
			if a.Deploy.TokenEnv == "" {
				a.Deploy.TokenEnv = "GITHUB_TOKEN"
			}
		}
	}
}

// normalizeNodePM fuerza pnpm en apps Node: comando npm -> pnpm y pasos
// npm/npx -> pnpm. Se aplica al cargar y al guardar: nunca ejecutamos npm.
func normalizeNodePM(a *App) {
	base := strings.ToLower(a.Command)
	if i := strings.LastIndexAny(base, `\/`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".cmd"), ".exe")
	if base == "npm" {
		a.Command = "pnpm"
	}
	if a.Deploy == nil {
		return
	}
	for i, step := range a.Deploy.Build {
		a.Deploy.Build[i] = pnpmStep(step)
	}
}

func pnpmStep(step string) string {
	t := strings.TrimSpace(step)
	low := strings.ToLower(t)
	switch {
	case low == "npm ci" || low == "npm i" || low == "npm install",
		strings.HasPrefix(low, "npm ci "), strings.HasPrefix(low, "npm i "), strings.HasPrefix(low, "npm install "):
		return "pnpm install"
	case strings.HasPrefix(low, "npm run "):
		return "pnpm run " + t[len("npm run "):]
	case strings.HasPrefix(low, "npx "):
		return "pnpm exec " + t[len("npx "):]
	case strings.HasPrefix(low, "npm "):
		return "pnpm " + t[len("npm "):]
	}
	return step
}

// ResolveEnv fusiona env_file + inline con expansión. Una sola vez por
// valor: Load la corre tras Validate; el snapshot DB no (DB guarda
// ya expandido). Soporta $$ como $ literal.
func (a *App) ResolveEnv() error {
	fileVars := map[string]string{}
	if a.EnvFile != "" {
		raw, err := dotenv.ParseFile(a.EnvFile)
		if err != nil {
			return fmt.Errorf("apps[%s]: %w", a.Name, err)
		}
		fileVars = dotenv.Expand(raw, dotenv.EnvironMap())
	}
	scope := dotenv.EnvironMap()
	for k, v := range fileVars {
		scope[k] = v
	}
	merged := make(map[string]string, len(fileVars)+len(a.Env))
	for k, v := range fileVars {
		merged[k] = v
	}
	for k, v := range dotenv.Expand(a.Env, scope) {
		merged[k] = v
	}
	a.Env = merged
	return nil
}

// ResolveEnvAll fusiona env_file+inline de todas las apps (una sola vez;
// el snapshot DB no re-fusiona: DB guarda ya expandido).
func (cfg *Config) ResolveEnvAll() error {
	for i := range cfg.Apps {
		if err := cfg.Apps[i].ResolveEnv(); err != nil {
			return err
		}
	}
	return nil
}

// Validate verifica todo y parsea (grace, github, apps, routes).
// No toca env (ver ResolveEnvAll).
func (cfg *Config) Validate() error {
	if cfg.Healthz.Enabled && cfg.Healthz.Listen == "" {
		return fmt.Errorf("healthz sin listen")
	}
	// Fail-closed: healthz expone acciones sin auth (restart/deploy). Solo
	// loopback. Exponerlo en LAN/publicaría el control de la máquina.
	if cfg.Healthz.Enabled && !isLoopbackAddr(cfg.Healthz.Listen) {
		return fmt.Errorf("healthz.listen debe ser loopback (127.0.0.1:8090 o [::1]:8090), no %q", cfg.Healthz.Listen)
	}
	if cfg.RateLimit.Enabled && (cfg.RateLimit.RPS <= 0 || cfg.RateLimit.Burst < 1) {
		return fmt.Errorf("rate_limit inválido: rps>0 y burst>=1")
	}
	for _, raw := range cfg.RateLimit.Allow {
		if _, _, err := net.ParseCIDR(raw); err != nil {
			return fmt.Errorf("rate_limit.allow inválido %q (usa CIDR, ej 10.0.0.0/8)", raw)
		}
	}
	g, err := time.ParseDuration(cfg.RestartGrace)
	if err != nil || g < 0 {
		return fmt.Errorf("restart_grace inválido %q (ej 10s, 0 = directo a 502)", cfg.RestartGrace)
	}
	cfg.grace = g
	if cfg.MaxBodyMB < 0 {
		return fmt.Errorf("max_body_mb inválido %d (>=0, 0 = sin tope)", cfg.MaxBodyMB)
	}
	if cfg.ACMEMaxNewHosts < -1 {
		return fmt.Errorf("acme_max_new_hosts inválido %d (>=-1, -1 = sin tope)", cfg.ACMEMaxNewHosts)
	}
	if cfg.Github != nil {
		gh := cfg.Github
		if gh.AppID <= 0 {
			return fmt.Errorf("github sin app_id")
		}
		p, err := time.ParseDuration(gh.PollInterval)
		if err != nil || p < 10*time.Second {
			return fmt.Errorf("github poll_interval inválido %q (mín 10s)", gh.PollInterval)
		}
		gh.poll = p
		if _, err := os.Stat(gh.KeyFile); err != nil {
			return fmt.Errorf("github key_file inaccesible %q", gh.KeyFile)
		}
		if _, err := github.New(gh.AppID, gh.KeyFile, ""); err != nil {
			return fmt.Errorf("github: %w", err)
		}
		if gh.WebhookSecret != "" && len(gh.WebhookSecret) < 16 {
			return fmt.Errorf("github webhook_secret muy corto (mín 16 caracteres)")
		}
	}
	for _, p := range cfg.DenyPaths {
		if _, err := pathpkg.Match(p, ""); err != nil {
			return fmt.Errorf("deny_paths patrón inválido %q", p)
		}
	}
	for _, c := range cfg.TLSCerts {
		if len(c.Hosts) == 0 || c.CertFile == "" || c.KeyFile == "" {
			return fmt.Errorf("tls_certs incompleto: hosts, cert_file y key_file")
		}
		if _, err := os.Stat(c.CertFile); err != nil {
			return fmt.Errorf("tls_certs cert_file inaccesible %q", c.CertFile)
		}
		if _, err := os.Stat(c.KeyFile); err != nil {
			return fmt.Errorf("tls_certs key_file inaccesible %q", c.KeyFile)
		}
		for _, h := range c.Hosts {
			h = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h)), "*.")
			if h == "" || strings.ContainsAny(h, " /\\") {
				return fmt.Errorf("tls_certs host inválido %q", h)
			}
		}
	}
	seenApps := map[string]bool{}
	seenPorts := map[string]string{}
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if seenApps[a.Name] {
			return fmt.Errorf("apps name duplicado %q", a.Name)
		}
		seenApps[a.Name] = true
		if p := a.Env["PORT"]; p != "" {
			if prev, ok := seenPorts[p]; ok {
				return fmt.Errorf("apps %q y %q comparten puerto %s", prev, a.Name, p)
			}
			seenPorts[p] = a.Name
		}
		if err := a.ValidateApp(i); err != nil {
			return err
		}
	}
	return ValidateRoutes(cfg.Routes)
}

// ValidateRoutes hosts duplicados, targets resolvibles. Canónica;
// Validate y cfgfile.Snapshot la usan (antes 2 copias).
func ValidateRoutes(routes []Route) error {
	seen := map[string]string{}
	for i := range routes {
		r := &routes[i]
		if len(r.Hosts) == 0 {
			return fmt.Errorf("route %d sin hosts", i)
		}
		if r.Target == "" {
			return fmt.Errorf("route %d sin target", i)
		}
		if err := r.Resolve(); err != nil {
			return fmt.Errorf("route %d %w", i, err)
		}
		for _, h := range r.Hosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if h == "" {
				return fmt.Errorf("route %d con host vacío", i)
			}
			if prev, ok := seen[h]; ok {
				return fmt.Errorf("host %q duplicado (en %q y %q)", h, prev, r.Target)
			}
			seen[h] = r.Target
		}
	}
	return nil
}

// Clone devuelve una copia profunda y editable de la config. La config viva
// se comparte entre goroutines sin lock: toda mutación pasa por una copia.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	out := *c
	out.DenyPaths = append([]string(nil), c.DenyPaths...)
	out.TLSCerts = make([]TLSCert, len(c.TLSCerts))
	for i, t := range c.TLSCerts {
		out.TLSCerts[i] = t
		out.TLSCerts[i].Hosts = append([]string(nil), t.Hosts...)
	}
	if c.GlobalEnv != nil {
		out.GlobalEnv = make(map[string]string, len(c.GlobalEnv))
		for k, v := range c.GlobalEnv {
			out.GlobalEnv[k] = v
		}
	}
	if c.Github != nil {
		g := *c.Github
		out.Github = &g
	}
	out.Apps = make([]App, len(c.Apps))
	for i, a := range c.Apps {
		out.Apps[i] = a
		out.Apps[i].Args = append([]string(nil), a.Args...)
		out.Apps[i].Cron = append([]CronJob(nil), a.Cron...)
		out.Apps[i].Persist = append([]string(nil), a.Persist...)
		out.Apps[i].AllowIPs = append([]string(nil), a.AllowIPs...)
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
			d.Release = append([]string(nil), a.Deploy.Release...)
			out.Apps[i].Deploy = &d
		}
	}
	out.Routes = make([]Route, len(c.Routes))
	for i, r := range c.Routes {
		out.Routes[i] = r
		out.Routes[i].Hosts = append([]string(nil), r.Hosts...)
	}
	return &out
}

// Live es la config activa con swap atómico para hot-reload.
// Los handlers leen Get() por request; el watcher hace Swap().
type Live struct {
	p atomic.Pointer[Config]
}

// NewLive parte de una config ya validada.
func NewLive(c *Config) *Live {
	l := &Live{}
	l.p.Store(c)
	return l
}

// Get nunca es nil tras NewLive.
func (l *Live) Get() *Config { return l.p.Load() }

// Swap reemplaza la config activa (ya validada).
func (l *Live) Swap(c *Config) { l.p.Store(c) }

// NeedRestart lista campos que exigen reinicio (no recargables en
// caliente: listeners, ACME, healthz, rate-limit).
func NeedRestart(old, new *Config) []string {
	out := []string{}
	if old.ListenHTTP != new.ListenHTTP {
		out = append(out, "listen_http")
	}
	if old.ListenHTTPS != new.ListenHTTPS {
		out = append(out, "listen_https")
	}
	if old.ACMEEmail != new.ACMEEmail {
		out = append(out, "acme_email")
	}
	if old.ACMEStaging != new.ACMEStaging {
		out = append(out, "acme_staging")
	}
	if old.ACMECacheDir != new.ACMECacheDir {
		out = append(out, "acme_cache_dir")
	}
	if old.ACMEMaxNewHosts != new.ACMEMaxNewHosts || old.ACMEDNSPrecheck != new.ACMEDNSPrecheck {
		out = append(out, "acme_limites")
	}
	if len(old.TLSCerts) != len(new.TLSCerts) {
		out = append(out, "tls_certs")
	}
	if old.Healthz != new.Healthz {
		out = append(out, "healthz")
	}
	if !rateLimitEqual(old.RateLimit, new.RateLimit) {
		out = append(out, "rate_limit")
	}
	if !githubEqual(old.Github, new.Github) {
		out = append(out, "github")
	}
	return out
}

// rateLimitEqual compara también la allowlist (slice no comparable).
func rateLimitEqual(a, b RateLimitConfig) bool {
	if a.Enabled != b.Enabled || a.RPS != b.RPS || a.Burst != b.Burst ||
		len(a.Allow) != len(b.Allow) {
		return false
	}
	for i := range a.Allow {
		if a.Allow[i] != b.Allow[i] {
			return false
		}
	}
	return true
}

// githubEqual compara bloque github (nil = ausente).
func githubEqual(a, b *Github) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.AppID == b.AppID && a.AppSlug == b.AppSlug &&
		a.KeyFile == b.KeyFile && a.PollInterval == b.PollInterval &&
		a.WebhookSecret == b.WebhookSecret
}

// ValidateApp verifica una app aislada (para escritura en DB).
func (a *App) ValidateApp(idx int) error {
	if !ValidAppName(a.Name) {
		return fmt.Errorf("apps[%d] name inválido %q: [a-z0-9-]", idx, a.Name)
	}
	switch a.Runtime {
	case RuntimeManaged:
		if strings.TrimSpace(a.Command) == "" {
			return fmt.Errorf("apps[%s] sin command", a.Name)
		}
	case RuntimeExternal:
		if strings.TrimSpace(a.Command) != "" || len(a.Args) > 0 || a.Static != "" || a.Deploy != nil {
			return fmt.Errorf("apps[%s] externa no admite command/args/static/deploy", a.Name)
		}
		if strings.TrimSpace(a.Dir) == "" {
			return fmt.Errorf("apps[%s] externa requiere dir", a.Name)
		}
		if strings.TrimSpace(a.Env["PORT"]) == "" {
			return fmt.Errorf("apps[%s] externa requiere PORT", a.Name)
		}
	default:
		return fmt.Errorf("apps[%s] runtime inválido %q (managed|external)", a.Name, a.Runtime)
	}
	if a.Deploy != nil && a.Dir == "" {
		return fmt.Errorf("apps[%s] deploy requiere dir", a.Name)
	}
	if a.Dir != "" && a.Deploy == nil {
		if st, err := os.Stat(a.Dir); err != nil || !st.IsDir() {
			return fmt.Errorf("apps[%s] dir inexistente %q", a.Name, a.Dir)
		}
	}
	if a.HealthPath != "" {
		if !strings.HasPrefix(a.HealthPath, "/") || strings.ContainsAny(a.HealthPath, " \t\r\n") {
			return fmt.Errorf("apps[%s] health_path inválido %q (debe empezar por /)", a.Name, a.HealthPath)
		}
	}
	for _, p := range a.Persist {
		clean := pathpkg.Clean(strings.TrimSpace(p))
		if p == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") ||
			pathpkg.IsAbs(clean) || strings.ContainsAny(clean, " \t\r\n") {
			return fmt.Errorf("apps[%s] persist inválido %q (ruta relativa dentro de la app)", a.Name, p)
		}
	}
	if a.BasicAuth != "" {
		user, hash, ok := strings.Cut(a.BasicAuth, ":")
		if !ok || user == "" || !strings.HasPrefix(hash, "$2") {
			return fmt.Errorf("apps[%s] basic_auth inválido: usa usuario:<hash bcrypt>", a.Name)
		}
	}
	for _, raw := range a.AllowIPs {
		if _, _, err := net.ParseCIDR(raw); err != nil {
			return fmt.Errorf("apps[%s] allow_ips inválido %q (usa CIDR)", a.Name, raw)
		}
	}
	d, err := time.ParseDuration(a.RestartDelay)
	if err != nil || d <= 0 {
		return fmt.Errorf("apps[%s] restart_delay inválido %q (ej 5s)", a.Name, a.RestartDelay)
	}
	a.delay = d
	mu, err := time.ParseDuration(a.MinUptime)
	if err != nil || mu <= 0 {
		return fmt.Errorf("apps[%s] min_uptime inválido %q (ej 10s)", a.Name, a.MinUptime)
	}
	a.minUp = mu
	if a.Deploy != nil {
		d := a.Deploy
		if strings.TrimSpace(d.Repo) == "" {
			return fmt.Errorf("apps[%s] deploy sin repo", a.Name)
		}
		for _, s := range d.Build {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("apps[%s] deploy con build vacío", a.Name)
			}
		}
		for _, s := range d.Release {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("apps[%s] deploy con release vacío", a.Name)
			}
		}
	}
	seenCron := map[string]bool{}
	for _, job := range a.Cron {
		if !ValidCronName(job.Name) {
			return fmt.Errorf("apps[%s] cron name inválido %q: [a-z0-9-]", a.Name, job.Name)
		}
		if seenCron[job.Name] {
			return fmt.Errorf("apps[%s] cron duplicado %q", a.Name, job.Name)
		}
		seenCron[job.Name] = true
		if _, err := ParseCron(job.Schedule); err != nil {
			return fmt.Errorf("apps[%s] cron %q: %w", a.Name, job.Name, err)
		}
		switch job.Kind() {
		case CronCommand:
			if strings.TrimSpace(job.Command) == "" {
				return fmt.Errorf("apps[%s] cron %q sin command", a.Name, job.Name)
			}
			if strings.TrimSpace(job.Path) != "" || strings.TrimSpace(job.Method) != "" {
				return fmt.Errorf("apps[%s] cron %q: command no admite path/method", a.Name, job.Name)
			}
		case CronHTTP:
			if strings.TrimSpace(job.Command) != "" {
				return fmt.Errorf("apps[%s] cron %q: http no admite command", a.Name, job.Name)
			}
			path := strings.TrimSpace(job.Path)
			if !strings.HasPrefix(path, "/") || strings.Contains(path, "://") ||
				strings.ContainsAny(path, " \t\r\n") {
				return fmt.Errorf("apps[%s] cron %q: path inválido %q (usa /ruta)", a.Name, job.Name, job.Path)
			}
			if m := job.HTTPMethod(); m != "GET" && m != "POST" {
				return fmt.Errorf("apps[%s] cron %q: método %q no soportado (GET|POST)", a.Name, job.Name, m)
			}
		default:
			return fmt.Errorf("apps[%s] cron %q: tipo %q inválido (command|http)", a.Name, job.Name, job.Type)
		}
		if _, err := job.CronTimeout(); err != nil {
			return fmt.Errorf("apps[%s] cron %q: %w", a.Name, job.Name, err)
		}
	}
	return nil
}

// ValidAppName [a-z0-9-] para nombres de archivo de log seguros.
// Único hogar: CLI, API y MCP validan con esta regla.
func ValidAppName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// AppForTarget devuelve la app cuyo puerto sirve target ("" si no es app).
func (c *Config) AppForTarget(target string) *App {
	for i := range c.Apps {
		port := c.Apps[i].Env["PORT"]
		if port != "" && "http://127.0.0.1:"+port == target {
			return &c.Apps[i]
		}
	}
	return nil
}

// Match resuelve Host -> backend. Exacto primero, luego *.dominio.
func (c *Config) Match(host string) *url.URL {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	for i := range c.Routes {
		for _, h := range c.Routes[i].Hosts {
			if strings.ToLower(h) == host {
				return c.Routes[i].targetURL
			}
		}
	}
	for i := range c.Routes {
		for _, h := range c.Routes[i].Hosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if strings.HasPrefix(h, "*.") {
				suffix := h[1:] // .ejemplo.com
				if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
					return c.Routes[i].targetURL
				}
			}
		}
	}
	return nil
}

// isLoopbackAddr true si el listen es exclusivamente loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
