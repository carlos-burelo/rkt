// Package config: carga y validación de la config YAML canónica.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Route mapea uno o varios hosts al mismo backend.
type Route struct {
	Hosts  []string `yaml:"hosts"`
	Target string   `yaml:"target"`

	targetURL *url.URL `yaml:"-"`
}

// TargetURL backend ya parseado.
func (r *Route) TargetURL() *url.URL { return r.targetURL }

// Config canónica. Un solo camino, nada legacy.
type Config struct {
	ListenHTTP          string  `yaml:"listen_http"`
	ListenHTTPS         string  `yaml:"listen_https"`
	ACMEEmail           string  `yaml:"acme_email"`
	ACMEStaging         bool    `yaml:"acme_staging"`
	ACMECacheDir        string  `yaml:"acme_cache_dir"`
	RedirectHTTPToHTTPS bool    `yaml:"redirect_http_to_https"`
	Routes              []Route `yaml:"routes"`
}

func defaultConfig() Config {
	return Config{
		ListenHTTP:          ":80",
		ListenHTTPS:         ":443",
		ACMECacheDir:        `C:\ProgramData\rkt-proxy\certs`,
		RedirectHTTPToHTTPS: true,
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
