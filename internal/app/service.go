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
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/server"
)

// Program es el servicio: guarda servidores para Stop limpio.
type Program struct {
	cfgPath string
	log     *slog.Logger
	srvHTTP *http.Server
	srvTLS  *http.Server
	mgr     *autocert.Manager
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

// Stop apaga ambos servidores con timeout.
func (p *Program) Stop(_ service.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.srvTLS != nil {
		_ = p.srvTLS.Shutdown(ctx)
	}
	if p.srvHTTP != nil {
		_ = p.srvHTTP.Shutdown(ctx)
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
	p.srvHTTP = server.NewHTTP(cfg, p.mgr, backend)
	p.srvTLS = server.NewHTTPS(cfg, p.mgr, backend)

	p.log.Info("arrancando",
		"http", cfg.ListenHTTP,
		"https", cfg.ListenHTTPS,
		"routes", len(cfg.Routes),
		"email", cfg.ACMEEmail,
	)

	go func() {
		if err := p.srvHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.log.Error("http muerto", "err", err)
		}
	}()
	if err := p.srvTLS.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		p.log.Error("https muerto", "err", err)
		os.Exit(1)
	}
}
