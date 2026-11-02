// Package app orquesta el binario: carga config, arma handler,
// ACME y servidores. Implementa service.Interface para Windows.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/accesslog"
	"rkt-proxy/internal/acme"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/server"
	"rkt-proxy/internal/supervisor"
)

// Version la inyecta CI con -X en cada tag. Local: "dev".
var Version = "dev"

// pollInterval revisa el archivo de config (mtime+tamaño).
// Poll y no fsnotify: portable, sin deps, suficiente a 10s.
const pollInterval = 10 * time.Second

// Program es el servicio: guarda servidores para Stop limpio.
type Program struct {
	cfgPath   string
	log       *slog.Logger
	srvHTTP   *http.Server
	srvTLS    *http.Server
	srvHealth *http.Server
	alog      *accesslog.Logger
	sup       *supervisor.Manager
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
	if p.sup != nil {
		p.sup.Stop()
	}
	if p.alog != nil {
		_ = p.alog.Close()
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
	live := config.NewLive(cfg)

	p.sup = supervisor.New(cfg.Apps, filepath.Join(logx.DirFor(p.cfgPath), "apps"), p.log)
	p.sup.Start()
	backend := proxy.NewHandler(live, p.log)
	handler := http.Handler(backend)
	if alog, err := accesslog.New(logx.AccessPath(p.cfgPath)); err != nil {
		p.log.Warn("access log desactivado", "err", err)
	} else {
		p.alog = alog
		handler = alog.Middleware(backend)
	}
	p.mgr = acme.NewManager(live, p.log)
	challenge := p.mgr.HTTPHandler(nil) // eager: activa http-01 como fallback
	errLog := slog.NewLogLogger(p.log.Handler(), slog.LevelError)
	p.srvHTTP = server.NewHTTP(cfg, live, challenge, handler, errLog)
	p.srvTLS = server.NewHTTPS(cfg, p.mgr, handler, errLog)

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
			Handler:      health.NewHandler(health.Deps{Live: live, Version: Version, Apps: p.appStatuses, Restart: p.sup.Restart}),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}
		go func() {
			if err := p.srvHealth.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.log.Error("healthz muerto", "err", err)
			}
		}()
	}
	p.log.Info("hot-reload activo", "poll", pollInterval.String())
	go p.watchConfig(live)
	if err := p.srvTLS.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		p.log.Error("https muerto", "err", err)
		os.Exit(1)
	}
}

// appStatuses adapta supervisor a health (nil si no hay manager).
func (p *Program) appStatuses() []health.AppStatus {
	if p.sup == nil {
		return nil
	}
	out := []health.AppStatus{}
	for _, s := range p.sup.Snapshot() {
		out = append(out, health.AppStatus{
			Name: s.Name, Running: s.Running, Pid: s.Pid,
			Restarts: s.Restarts, Failed: s.Failed,
		})
	}
	return out
}

// watchConfig recarga rutas + redirect sin downtime ante cambios del
// archivo. Config inválida = se conserva la anterior. Campos que exigen
// reinicio (listeners, ACME, healthz, rate-limit) solo se avisan.
func (p *Program) watchConfig(live *config.Live) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	var lastTime int64
	var lastSize int64
	if st, err := os.Stat(p.cfgPath); err == nil {
		lastTime, lastSize = st.ModTime().UnixNano(), st.Size()
	}
	for range t.C {
		st, err := os.Stat(p.cfgPath)
		if err != nil {
			continue
		}
		if st.ModTime().UnixNano() == lastTime && st.Size() == lastSize {
			continue
		}
		lastTime, lastSize = st.ModTime().UnixNano(), st.Size()
		next, err := config.Load(p.cfgPath)
		if err != nil {
			p.log.Warn("config nueva inválida, conservo anterior", "err", err)
			continue
		}
		old := live.Get()
		live.Swap(next)
		p.log.Info("config recargada",
			"routes", len(next.Routes),
			"redirect", next.RedirectHTTPToHTTPS,
		)
		if need := config.NeedRestart(old, next); len(need) > 0 {
			p.log.Warn("cambios requieren restart", "campos", need)
		}
	}
}
