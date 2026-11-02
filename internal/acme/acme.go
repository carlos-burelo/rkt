// Package acme construye el Manager autocert (Let's Encrypt)
// con HostPolicy derivada de la config: solo hosts configurados.
package acme

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	leacme "golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/config"
)

// StagingDirectory evita rate-limit de prod en pruebas.
const StagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"

// HostPolicy acepta hosts exactos de la config viva y
// subdominios de bases wildcard (*.dominio). La base sola
// pasa solo si está listada explícita (igual que Match).
func HostPolicy(live *config.Live) autocert.HostPolicy {
	return func(_ context.Context, host string) error {
		host = strings.ToLower(strings.TrimSpace(host))
		cfg := live.Get()
		for i := range cfg.Routes {
			for _, h := range cfg.Routes[i].Hosts {
				h = strings.ToLower(strings.TrimSpace(h))
				if strings.HasPrefix(h, "*.") {
					if strings.HasSuffix(host, "."+strings.TrimPrefix(h, "*.")) {
						return nil
					}
				} else if h == host {
					return nil
				}
			}
		}
		return fmt.Errorf("host %q no permitido", host)
	}
}

// NewManager crea el Manager con cache en disco y TOS aceptado.
// Staging y prod usan subdirs distintos: el cert staging cacheado
// jamás debe servirse como prod (ni viceversa).
func NewManager(live *config.Live, log *slog.Logger) *autocert.Manager {
	cfg := live.Get()
	dir := filepath.Join(cfg.ACMECacheDir, "prod")
	if cfg.ACMEStaging {
		dir = filepath.Join(cfg.ACMECacheDir, "staging")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Warn("no se pudo crear cache dir", "err", err)
	}
	m := &autocert.Manager{
		Cache:      autocert.DirCache(dir),
		Prompt:     autocert.AcceptTOS,
		Email:      cfg.ACMEEmail,
		HostPolicy: HostPolicy(live),
	}
	if cfg.ACMEStaging {
		m.Client = &leacme.Client{DirectoryURL: StagingDirectory}
		log.Warn("usando Let's Encrypt STAGING")
	}
	return m
}
