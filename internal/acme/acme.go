// Package acme construye el Manager autocert (Let's Encrypt)
// con HostPolicy derivada de la config: solo hosts configurados.
package acme

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	leacme "golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/time/rate"

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
	policy := HostPolicy(live)
	if cfg.ACMEDNSPrecheck {
		policy = dnsPolicy(policy)
	}
	policy = limitPolicy(cfg.ACMEMaxNewHosts, policy)
	m := &autocert.Manager{
		Cache:      autocert.DirCache(dir),
		Prompt:     autocert.AcceptTOS,
		Email:      cfg.ACMEEmail,
		HostPolicy: policy,
	}
	if cfg.ACMEStaging {
		m.Client = &leacme.Client{DirectoryURL: StagingDirectory}
		log.Warn("usando Let's Encrypt STAGING")
	}
	return m
}

// limitPolicy limita emisiones nuevas por hora. autocert solo llama a
// HostPolicy cuando no hay certificado cacheado, así que los hosts ya
// emitidos no consumen cupo. max < 0 desactiva el límite.
func limitPolicy(max int, next autocert.HostPolicy) autocert.HostPolicy {
	if max <= 0 {
		return next
	}
	lim := rate.NewLimiter(rate.Every(time.Hour/time.Duration(max)), max)
	var mu sync.Mutex
	seen := map[string]struct{}{}
	return func(ctx context.Context, host string) error {
		if err := next(ctx, host); err != nil {
			return err
		}
		host = strings.ToLower(strings.TrimSpace(host))
		mu.Lock()
		if _, ok := seen[host]; ok {
			mu.Unlock()
			return nil
		}
		mu.Unlock()
		if !lim.Allow() {
			return fmt.Errorf("límite de emisión ACME alcanzado (acme_max_new_hosts); reintenta más tarde")
		}
		mu.Lock()
		if len(seen) > 4096 {
			seen = map[string]struct{}{}
		}
		seen[host] = struct{}{}
		mu.Unlock()
		return nil
	}
}

// dnsPolicy exige que el host resuelva antes de pedir certificado: evita
// gastar intentos ACME con subdominios random.
func dnsPolicy(next autocert.HostPolicy) autocert.HostPolicy {
	return func(ctx context.Context, host string) error {
		if err := next(ctx, host); err != nil {
			return err
		}
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil || len(ips) == 0 {
			return fmt.Errorf("host %q no resuelve por DNS", host)
		}
		return nil
	}
}
