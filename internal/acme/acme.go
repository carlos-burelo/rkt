// Package acme construye el Manager autocert (Let's Encrypt)
// con HostPolicy derivada de la config: solo hosts configurados.
package acme

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	leacme "golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/config"
)

// StagingDirectory evita rate-limit de prod en pruebas.
const StagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"

// HostPolicy acepta hosts exactos de la config y
// subdominios de bases wildcard (*.dominio). La base sola
// pasa solo si está listada explícita (igual que Match).
func HostPolicy(cfg *config.Config) autocert.HostPolicy {
	exact := map[string]bool{}
	bases := []string{}
	for _, r := range cfg.Routes {
		for _, h := range r.Hosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if strings.HasPrefix(h, "*.") {
				bases = append(bases, strings.TrimPrefix(h, "*."))
			} else if h != "" {
				exact[h] = true
			}
		}
	}
	return func(_ context.Context, host string) error {
		host = strings.ToLower(strings.TrimSpace(host))
		if exact[host] {
			return nil
		}
		for _, base := range bases {
			if strings.HasSuffix(host, "."+base) {
				return nil
			}
		}
		return fmt.Errorf("host %q no permitido", host)
	}
}

// NewManager crea el Manager con cache en disco y TOS aceptado.
func NewManager(cfg *config.Config, log *slog.Logger) *autocert.Manager {
	if err := os.MkdirAll(cfg.ACMECacheDir, 0o700); err != nil {
		log.Warn("no se pudo crear cache dir", "err", err)
	}
	m := &autocert.Manager{
		Cache:      autocert.DirCache(cfg.ACMECacheDir),
		Prompt:     autocert.AcceptTOS,
		Email:      cfg.ACMEEmail,
		HostPolicy: HostPolicy(cfg),
	}
	if cfg.ACMEStaging {
		m.Client = &leacme.Client{DirectoryURL: StagingDirectory}
		log.Warn("usando Let's Encrypt STAGING")
	}
	return m
}
