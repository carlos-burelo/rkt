// Package app orquesta el binario: carga config, arma handler,
// ACME y servidores. Implementa service.Interface para Windows.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/accesslog"
	"rkt-proxy/internal/acme"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/hook"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/server"
	"rkt-proxy/internal/store"
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
	live      *config.Live
	ghm       *github.Manager
	db        *store.DB
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
	if p.db != nil {
		_ = p.db.Close()
	}
	return nil
}

// Run carga config (YAML bootstrap + flota DB) y sirve. Bloquea en TLS.
func (p *Program) Run() {
	cfg, db, err := cfgfile.LoadMerged(p.cfgPath)
	if err != nil {
		p.log.Error("config inválida", "err", err)
		os.Exit(1)
	}
	p.db = db
	live := config.NewLive(cfg)
	p.live = live

	p.sup = supervisor.New(cfg.Apps, filepath.Join(logx.DirFor(p.cfgPath), "apps"), p.log)
	if p.db != nil {
		db := p.db
		p.sup.OnEvent = func(kind, app, msg string) {
			if err := db.InsertEvent(kind, app, msg); err != nil {
				p.log.Warn("evento no guardado", "err", err)
			}
		}
	}
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
	var control http.Handler
	if cfg.Github != nil && cfg.Github.WebhookSecret != "" {
		secret := cfg.Github.WebhookSecret
		control = hook.NewHandler(live, secret, func(name string) {
			go func() {
				if rev, err := p.deployApp(name); err != nil {
					p.log.Error("deploy por hook fallido", "app", name, "err", err)
				} else {
					p.log.Info("deploy por hook ok", "app", name, "rev", rev)
				}
			}()
		}, p.log)
	}
	p.srvHTTP = server.NewHTTP(cfg, live, challenge, handler, control, errLog)
	p.srvTLS = server.NewHTTPS(cfg, p.mgr, handler, control, errLog)

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
			Addr:    cfg.Healthz.Listen,
			Handler: health.NewHandler(health.Deps{Live: live, Version: Version, Apps: p.appStatuses, Restart: p.sup.Restart, Deploy: p.deployApp, Rollback: p.rollbackApp}),
			// Sin WriteTimeout: POST /deploy/* tarda minutos (solo loopback).
			ReadTimeout: 10 * time.Second,
		}
		go func() {
			if err := p.srvHealth.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.log.Error("healthz muerto", "err", err)
			}
		}()
	}
	p.log.Info("hot-reload activo", "poll", pollInterval.String())
	go p.watchMerged(live, db)
	if cfg.Github != nil {
		gh, err := github.New(cfg.Github.AppID, cfg.Github.InstallationID, cfg.Github.KeyFile, "")
		if err != nil {
			p.log.Error("github app inválida, sin auto-deploy", "err", err)
		} else {
			p.ghm = gh
			p.log.Info("auto-deploy activo", "poll", cfg.Github.Poll().String())
			go p.autoDeploy(live)
		}
	}
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

// supCtl adapta Manager a deploy.Controller.
type supCtl struct {
	sup *supervisor.Manager
	log *slog.Logger
}

func (c *supCtl) SwapAndRestart(name string, swap func() error) error {
	return c.sup.SwapAndRestart(name, swap)
}

func (c *supCtl) Snapshot() []supervisor.Status {
	return c.sup.Snapshot()
}

// deployApp ejecuta un deploy en el servicio: token del entorno del
// SERVICIO (variable de máquina para SYSTEM), log a archivo.
func (p *Program) deployApp(name string) (string, error) {
	cfg := p.live.Get()
	var app *config.App
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == name {
			app = &cfg.Apps[i]
		}
	}
	if app == nil {
		return "", fmt.Errorf("app %q inexistente", name)
	}
	if app.Deploy == nil {
		return "", fmt.Errorf("app %q sin bloque deploy", name)
	}
	token := os.Getenv(app.Deploy.TokenEnv)
	if deploy.NeedsAuth(app.Deploy.Repo) {
		if p.ghm != nil {
			var err error
			token, err = p.ghm.Token()
			if err != nil {
				return "", fmt.Errorf("token github app: %w", err)
			}
		} else if token == "" {
			return "", fmt.Errorf("falta token en env %s (variable de máquina para el servicio)", app.Deploy.TokenEnv)
		}
	}
	dir := logx.DirFor(p.cfgPath)
	if err := os.MkdirAll(filepath.Join(dir, "deploys"), 0o755); err != nil {
		return "", err
	}
	out, err := os.OpenFile(
		filepath.Join(dir, "deploys", name+"-"+time.Now().Format("20060102-150405")+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer out.Close()
	var depID int64
	prevRev := deploy.HeadRev(app.Dir)
	if p.db != nil {
		depID, _ = p.db.InsertDeployment(app.Name, "", out.Name())
	}
	rev, err := deploy.Run(*app, token, &supCtl{sup: p.sup, log: p.log}, out, p.log)
	if p.db != nil && depID > 0 {
		status := "ok"
		if err != nil {
			status = "failed"
		}
		if ferr := p.db.FinishDeployment(depID, status, prevRev, rev); ferr != nil {
			p.log.Warn("historial no actualizado", "err", ferr)
		}
	}
	if err != nil {
		return "", err
	}
	return rev, nil
}

// rollbackApp devuelve live.prev a live (rollback manual).
func (p *Program) rollbackApp(name string) error {
	cfg := p.live.Get()
	var app *config.App
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == name {
			app = &cfg.Apps[i]
		}
	}
	if app == nil {
		return fmt.Errorf("app %q inexistente", name)
	}
	return deploy.Rollback(*app, &supCtl{sup: p.sup, log: p.log}, p.log)
}

// autoDeploy sondea GitHub por cambios y despliega (push-to-deploy con
// delay de poll). El git local es el estado: si HEAD != remoto, deploy.
func (p *Program) autoDeploy(live *config.Live) {
	t := time.NewTicker(live.Get().Github.Poll())
	defer t.Stop()
	for range t.C {
		cfg := live.Get()
		if cfg.Github == nil || p.ghm == nil {
			return
		}
		if err := discover.Run(discover.Deps{
			GH: p.ghm, DB: p.db, Log: p.log,
			DeployFn: func(name string) (string, error) { return p.deployApp(name) },
		}); err != nil {
			p.log.Warn("discovery fallida", "err", err)
		}
		for i := range cfg.Apps {
			a := &cfg.Apps[i]
			if a.Deploy == nil || !a.Deploy.Auto {
				continue
			}
			owner, repo, ok := github.ParseRepo(a.Deploy.Repo)
			if !ok {
				p.log.Warn("auto-deploy: repo no parseable", "app", a.Name)
				continue
			}
			tok, err := p.ghm.Token()
			if err != nil {
				p.log.Warn("auto-deploy: sin token", "err", err)
				continue
			}
			remote, err := p.ghm.CommitSHA(owner, repo, a.Deploy.Ref, tok)
			if err != nil {
				p.log.Warn("auto-deploy: sin remoto", "app", a.Name, "err", err)
				continue
			}
			if local := deploy.HeadRev(a.Dir); local == "" || local == remote {
				continue
			}
			p.log.Info("auto-deploy: cambio detectado", "app", a.Name, "rev", shortSHA(remote))
			if rev, err := p.deployApp(a.Name); err != nil {
				p.log.Error("auto-deploy fallido", "app", a.Name, "err", err)
			} else {
				p.log.Info("auto-deploy ok", "app", a.Name, "rev", rev)
			}
		}
	}
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// watchMerged recarga sin downtime ante cambios del YAML (infra) o de
// la DB (flota vía CLI). Fuente inválida = se conserva la anterior.
// Campos que exigen reinicio solo se avisan.
func (p *Program) watchMerged(live *config.Live, db *store.DB) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	var lastTime, lastSize int64
	if st, err := os.Stat(p.cfgPath); err == nil {
		lastTime, lastSize = st.ModTime().UnixNano(), st.Size()
	}
	lastVer := db.ConfigVersion()
	base, err := config.Load(p.cfgPath)
	if err != nil {
		p.log.Warn("yaml ilegible para watcher, solo DB", "err", err)
	}
	for range t.C {
		changed := false
		if st, err := os.Stat(p.cfgPath); err == nil {
			if st.ModTime().UnixNano() != lastTime || st.Size() != lastSize {
				lastTime, lastSize = st.ModTime().UnixNano(), st.Size()
				if b, err := config.Load(p.cfgPath); err != nil {
					p.log.Warn("yaml nuevo inválido, conservo anterior", "err", err)
				} else {
					base = b
					changed = true
				}
			}
		}
		if v := db.ConfigVersion(); v != lastVer {
			lastVer = v
			changed = true
		}
		if !changed {
			continue
		}
		next, err := cfgfile.Snapshot(db, base)
		if err != nil {
			p.log.Warn("snapshot inválido, conservo anterior", "err", err)
			continue
		}
		old := live.Get()
		live.Swap(next)
		p.log.Info("config recargada",
			"routes", len(next.Routes),
			"apps", len(next.Apps),
			"redirect", next.RedirectHTTPToHTTPS,
		)
		if need := config.NeedRestart(old, next); len(need) > 0 {
			p.log.Warn("cambios requieren restart", "campos", need)
		}
	}
}
