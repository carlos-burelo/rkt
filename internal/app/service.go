// Package app orquesta el binario: carga config, arma handler,
// ACME y servidores. Implementa service.Interface para Windows.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/acme"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/server"
)

// Version la inyecta CI con -X en cada tag. Local: "dev".
var Version = "dev"

// Program es el servicio: guarda servidores para Stop limpio.
type Program struct {
	cfgPath   string
	log       *slog.Logger
	srvHTTP   *http.Server
	srvTLS    *http.Server
	srvHealth *http.Server
	mgr       *autocert.Manager
}

// New crea el programa sin arrancarlo.
func New(cfgPath string, log *slog.Logger) *Program {
	return &Program{cfgPath: cfgPath, log: log}
}

// Start lanza Run en goroutine (contrato kardianos/service).
func (p *Program) Start(_ service.Service) error {
	go p.Run()
	return nil
}

// Stop apaga todos los servidores con timeout.
func (p *Program) Stop(_ service.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.srvTLS != nil {
		_ = p.srvTLS.Shutdown(ctx)
	}
	if p.srvHTTP != nil {
		_ = p.srvHTTP.Shutdown(ctx)
	}
	if p.srvHealth != nil {
		_ = p.srvHealth.Shutdown(ctx)
	}
	return nil
}

// Run carga config y sirve HTTP+HTTPS. Bloquea en el servidor TLS.
func (p *Program) Run() {
	cfg, err := config.Load(p.cfgPath)
	if err != nil {
		p.log.Error("config inválida", "err", err)
		os.Exit(1)
	}

	backend := proxy.NewHandler(cfg, p.log)
	p.mgr = acme.NewManager(cfg, p.log)
	challenge := p.mgr.HTTPHandler(nil) // eager: activa http-01 como fallback
	errLog := slog.NewLogLogger(p.log.Handler(), slog.LevelError)
	p.srvHTTP = server.NewHTTP(cfg, challenge, backend, errLog)
	p.srvTLS = server.NewHTTPS(cfg, p.mgr, backend, errLog)

	p.log.Info("arrancando",
		"http", cfg.ListenHTTP,
		"https", cfg.ListenHTTPS,
		"healthz", cfg.Healthz.Listen,
		"routes", len(cfg.Routes),
		"email", cfg.ACMEEmail,
	)

	go func() {
		if err := p.srvHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.log.Error("http muerto", "err", err)
		}
	}()
	if cfg.Healthz.Enabled {
		p.srvHealth = &http.Server{
			Addr:         cfg.Healthz.Listen,
			Handler:      health.NewHandler(cfg, Version),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}
		go func() {
			if err := p.srvHealth.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.log.Error("healthz muerto", "err", err)
			}
		}()
	}
	if err := p.srvTLS.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		p.log.Error("https muerto", "err", err)
		os.Exit(1)
	}
}
