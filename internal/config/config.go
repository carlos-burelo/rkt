// Package config: carga y validación de la config YAML canónica.
package config

import (
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"

	"rkt-proxy/internal/dotenv"
)

// Route mapea uno o varios hosts al mismo backend.
type Route struct {
	Hosts  []string `yaml:"hosts"`
	Target string   `yaml:"target"`

	targetURL *url.URL `yaml:"-"`
}

// TargetURL backend ya parseado.
func (r *Route) TargetURL() *url.URL { return r.targetURL }

// App es un proceso supervisado (mini-PaaS). El proxy enruta
// a sus puertos; rkt los levanta y reinicia si mueren.
type App struct {
	Name         string            `yaml:"name"`
	Command      string            `yaml:"command"`
	Args         []string          `yaml:"args"`
	Dir          string            `yaml:"dir"`
	EnvFile      string            `yaml:"env_file"`
	Env          map[string]string `yaml:"env"`
	RestartDelay string            `yaml:"restart_delay"`
	MinUptime    string            `yaml:"min_uptime"`
	MemoryMB     uint64            `yaml:"memory_mb"`

	delay time.Duration `yaml:"-"`
	minUp time.Duration `yaml:"-"`
}

// Delay espera base entre reinicios (parseado en Load).
func (a *App) Delay() time.Duration { return a.delay }

// MinUp vida mínima para no contar como crash (parseado en Load).
func (a *App) MinUp() time.Duration { return a.minUp }

// HealthzConfig expone salud en listener loopback dedicado
// (no compite con paths de backends ni se expone a internet).
type HealthzConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

// RateLimitConfig frena abuso por IP (bots) antes del backend.
type RateLimitConfig struct {
	Enabled bool    `yaml:"enabled"`
	RPS     float64 `yaml:"requests_per_second"`
	Burst   int     `yaml:"burst"`
}

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
	ListenHTTP          string          `yaml:"listen_http"`
	ListenHTTPS         string          `yaml:"listen_https"`
	ACMEEmail           string          `yaml:"acme_email"`
	ACMEStaging         bool            `yaml:"acme_staging"`
	ACMECacheDir        string          `yaml:"acme_cache_dir"`
	RedirectHTTPToHTTPS bool            `yaml:"redirect_http_to_https"`
	Healthz             HealthzConfig   `yaml:"healthz"`
	RateLimit           RateLimitConfig `yaml:"rate_limit"`
	RestartGrace        string          `yaml:"restart_grace"`
	DenyPaths           []string        `yaml:"deny_paths"`
	Apps                []App           `yaml:"apps"`
	Routes              []Route         `yaml:"routes"`

	grace time.Duration `yaml:"-"`
}

// Grace ventana de reintento ante backend caído (parseado en Load).
func (c *Config) Grace() time.Duration { return c.grace }

func defaultConfig() Config {
	return Config{
		ListenHTTP:          ":80",
		ListenHTTPS:         ":443",
		ACMECacheDir:        `C:\ProgramData\rkt-proxy\certs`,
		RedirectHTTPToHTTPS: true,
		Healthz:             HealthzConfig{Enabled: true, Listen: "127.0.0.1:8090"},
		RateLimit:           RateLimitConfig{Enabled: true, RPS: 20, Burst: 40},
		DenyPaths:           append([]string{}, defaultDenyPaths...),
	}
}

// Load lee y valida. Error = no arrancar.
func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leer config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if cfg.ListenHTTP == "" {
		cfg.ListenHTTP = ":80"
	}
	if cfg.ListenHTTPS == "" {
		cfg.ListenHTTPS = ":443"
	}
	if cfg.ACMECacheDir == "" {
		cfg.ACMECacheDir = `C:\ProgramData\rkt-proxy\certs`
	}
	if len(cfg.Routes) == 0 {
		return nil, fmt.Errorf("config sin routes")
	}
	if cfg.Healthz.Enabled && cfg.Healthz.Listen == "" {
		return nil, fmt.Errorf("healthz sin listen")
	}
	if cfg.RateLimit.Enabled && (cfg.RateLimit.RPS <= 0 || cfg.RateLimit.Burst < 1) {
		return nil, fmt.Errorf("rate_limit inválido: rps>0 y burst>=1")
	}
	if cfg.RestartGrace == "" {
		cfg.RestartGrace = restartGraceDefault
	}
	g, err := time.ParseDuration(cfg.RestartGrace)
	if err != nil || g < 0 {
		return nil, fmt.Errorf("restart_grace inválido %q (ej 10s, 0 = directo a 502)", cfg.RestartGrace)
	}
	cfg.grace = g
	for _, p := range cfg.DenyPaths {
		if _, err := pathpkg.Match(p, ""); err != nil {
			return nil, fmt.Errorf("deny_paths patrón inválido %q", p)
		}
	}
	seenApps := map[string]bool{}
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if !validAppName(a.Name) {
			return nil, fmt.Errorf("apps[%d] name inválido %q: [a-z0-9-]", i, a.Name)
		}
		if seenApps[a.Name] {
			return nil, fmt.Errorf("apps name duplicado %q", a.Name)
		}
		seenApps[a.Name] = true
		if strings.TrimSpace(a.Command) == "" {
			return nil, fmt.Errorf("apps[%s] sin command", a.Name)
		}
		if a.Dir != "" {
			if st, err := os.Stat(a.Dir); err != nil || !st.IsDir() {
				return nil, fmt.Errorf("apps[%s] dir inexistente %q", a.Name, a.Dir)
			}
		}
		if a.RestartDelay == "" {
			a.RestartDelay = "5s"
		}
		d, err := time.ParseDuration(a.RestartDelay)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("apps[%s] restart_delay inválido %q (ej 5s)", a.Name, a.RestartDelay)
		}
		a.delay = d
		if a.MinUptime == "" {
			a.MinUptime = "10s"
		}
		mu, err := time.ParseDuration(a.MinUptime)
		if err != nil || mu <= 0 {
			return nil, fmt.Errorf("apps[%s] min_uptime inválido %q (ej 10s)", a.Name, a.MinUptime)
		}
		a.minUp = mu
		// env: archivo base + inline encima, todo expandido.
		// file ve OS; inline ve OS+file+inline-anteriores (ordenado).
		fileVars := map[string]string{}
		if a.EnvFile != "" {
			raw, err := dotenv.ParseFile(a.EnvFile)
			if err != nil {
				return nil, fmt.Errorf("apps[%s]: %w", a.Name, err)
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
	}
	seen := map[string]string{}
	for i := range cfg.Routes {
		r := &cfg.Routes[i]
		if len(r.Hosts) == 0 {
			return nil, fmt.Errorf("route %d sin hosts", i)
		}
		if r.Target == "" {
			return nil, fmt.Errorf("route %d sin target", i)
		}
		u, err := url.Parse(r.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("route %d target inválido %q: debe ser http(s)://127.0.0.1:puerto", i, r.Target)
		}
		r.targetURL = u
		for _, h := range r.Hosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if h == "" {
				return nil, fmt.Errorf("route %d con host vacío", i)
			}
			if prev, ok := seen[h]; ok {
				return nil, fmt.Errorf("host %q duplicado (en %q y %q)", h, prev, r.Target)
			}
			seen[h] = r.Target
		}
	}
	return &cfg, nil
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
	if old.Healthz != new.Healthz {
		out = append(out, "healthz")
	}
	if old.RateLimit != new.RateLimit {
		out = append(out, "rate_limit")
	}
	if !appsEqual(old.Apps, new.Apps) {
		out = append(out, "apps")
	}
	return out
}

// validAppName [a-z0-9-] para nombres de archivo de log seguros.
func validAppName(s string) bool {
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

// appsEqual compara apps por nombre (orden irrelevante).
func appsEqual(a, b []App) bool {
	if len(a) != len(b) {
		return false
	}
	byName := map[string]App{}
	for _, x := range a {
		byName[x.Name] = x
	}
	for _, y := range b {
		x, ok := byName[y.Name]
		if !ok {
			return false
		}
		if x.Command != y.Command || x.Dir != y.Dir || x.delay != y.delay ||
			x.minUp != y.minUp || x.MemoryMB != y.MemoryMB ||
			len(x.Args) != len(y.Args) || len(x.Env) != len(y.Env) {
			return false
		}
		for i := range x.Args {
			if x.Args[i] != y.Args[i] {
				return false
			}
		}
		for k, v := range x.Env {
			if y.Env[k] != v {
				return false
			}
		}
	}
	return true
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

// AllowedHosts lista para autocert HostPolicy (sin *., con base).
func (c *Config) AllowedHosts() []string {
	out := []string{}
	for _, r := range c.Routes {
		for _, h := range r.Hosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if strings.HasPrefix(h, "*.") {
				out = append(out, strings.TrimPrefix(h, "*."))
			} else {
				out = append(out, h)
			}
		}
	}
	return out
}
