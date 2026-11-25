package app

import (
	"crypto/tls"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"rkt/internal/config"
	"rkt/internal/devca"
	"rkt/internal/logx"
	"rkt/internal/selfcert"
	"rkt/internal/store"
)

// dashboardTLS arma el TLS del listener propio del dashboard (dashboard_tls=auto).
//   - CA local (rkt dev): sus certificados, como el resto del edge.
//   - SNI == dashboard.host y ACME disponible: certificado Let's Encrypt de
//     ese dominio (la validación entra por :80, no por este puerto). Si
//     todavía no se puede emitir (DNS sin propagar), no rompe: cae al
//     autofirmado y lo registra.
//   - Cualquier otro nombre o acceso por IP: autofirmado persistente que
//     cubre localhost, la IP pública y el dominio; su huella queda en el log.
func (p *Program) dashboardTLS(cfg *config.Config, ca *devca.CA, mgr *autocert.Manager) (*tls.Config, error) {
	if ca != nil {
		return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: ca.GetCertificate}, nil
	}
	host := strings.ToLower(strings.TrimSpace(cfg.Dashboard.Host))
	names := selfcert.Names{
		Hosts: []string{"localhost", host},
		IPs:   []string{"127.0.0.1", "::1", strings.TrimSpace(p.db.GetSetting(store.KeyPublicIP))},
	}
	self, fp, err := selfcert.Load(filepath.Join(logx.SecretsDir(p.cfgPath), "dashboard-tls"), names, time.Now())
	if err != nil {
		return nil, err
	}
	p.log.Info("dashboard TLS activo", "listen", cfg.Dashboard.Listen, "dominio", host,
		"autofirmado_sha256", fp, "nota", "fija esta huella en clientes/agentes; con dominio y DNS correcto se usa Let's Encrypt")
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if mgr != nil && host != "" && strings.EqualFold(h.ServerName, host) {
				c, err := mgr.GetCertificate(h)
				if err == nil {
					return c, nil
				}
				p.log.Warn("dashboard TLS: sin certificado ACME aún, uso autofirmado", "host", host, "err", err)
			}
			return self, nil
		},
	}, nil
}
