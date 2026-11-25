// Package app orquesta el binario: carga config, arma handler,
// ACME y servidores. Implementa service.Interface (kardianos/service).
package app

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/crypto/acme/autocert"

	"rkt/internal/accesslog"
	"rkt/internal/acme"
	"rkt/internal/api"
	"rkt/internal/build"
	"rkt/internal/buildpack"
	"rkt/internal/canary"
	"rkt/internal/cfgfile"
	"rkt/internal/config"
	"rkt/internal/cronjobs"
	"rkt/internal/dashboard"
	"rkt/internal/deploy"
	"rkt/internal/devca"
	"rkt/internal/geoip"
	"rkt/internal/gitcred"
	"rkt/internal/github"
	"rkt/internal/health"
	"rkt/internal/hook"
	"rkt/internal/hub"
	"rkt/internal/logx"
	"rkt/internal/mcpserver"
	"rkt/internal/platform"
	"rkt/internal/proxy"
	"rkt/internal/ratelimit"
	"rkt/internal/server"
	"rkt/internal/servermetrics"
	"rkt/internal/store"
	"rkt/internal/supervisor"
	"rkt/web"
)

// Version vive en internal/build (inyectada por CI). Ver build.Version.

// pollInterval revisa el archivo de config (mtime+tamaño).
// Poll y no fsnotify: portable, sin deps, suficiente a 10s.
const pollInterval = 10 * time.Second

// Program es el servicio: guarda servidores para Stop limpio.
type Program struct {
	cfgPath      string
	geo          *geoip.Locator
	log          *slog.Logger
	live         *config.Live
	devMode      bool
	ghm          atomic.Pointer[github.Manager]
	ghStale      atomic.Bool
	autoStarted  atomic.Bool
	db           *store.DB
	discoverKick chan struct{}
	srvHTTP      *http.Server
	srvTLS       *http.Server
	srvHealth    *http.Server
	srvDashboard *http.Server
	alog         *accesslog.Logger
	sup          *supervisor.Manager
	mgr          *autocert.Manager
	devca        *devca.CA
	probeMu      sync.Mutex
	probes       map[string]probeResult
	// ops serializa operaciones destructivas por app (deploy/build/rollback).
	opMu sync.Mutex
	ops  map[string]bool
	// deployMu protege el registro de últimos deploys para auto-rollback.
	deployMu    sync.Mutex
	lastDeploy  map[string]time.Time
	rollingBack map[string]bool
	// cancelMu protege los canceladores de despliegues en curso.
	cancelMu      sync.Mutex
	deployCancels map[int64]context.CancelFunc
	// pendingMu protege los campos que exigen restart (deriva aplicada).
	pendingMu      sync.Mutex
	pendingRestart []string
	// srvMetrics monitor local de la instancia (compartido con API).
	srvMetrics *servermetrics.Monitor
	// cron ejecutor de tareas programadas.
	cron *cronjobs.Runner
	// hub bus de eventos para el stream vivo de la flota.
	hub *hub.Bus
	// stateMu sincroniza los recursos que Stop lee mientras Run los crea.
	stateMu sync.Mutex
}

// beginOp reclama la app para una operación destructiva. Devuelve error si
// ya hay otra en curso: dos deploys simultáneos corromperían live/prev.
func (p *Program) beginOp(name string) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	if p.ops == nil {
		p.ops = map[string]bool{}
	}
	if p.ops[name] {
		return fmt.Errorf("ya hay un deploy/build/rollback en curso para %q", name)
	}
	p.ops[name] = true
	return nil
}

func (p *Program) endOp(name string) {
	p.opMu.Lock()
	delete(p.ops, name)
	p.opMu.Unlock()
}

type probeResult struct {
	at time.Time
	ok bool
}

// New crea el programa sin arrancarlo.
func New(cfgPath string, log *slog.Logger) *Program {
	return &Program{cfgPath: cfgPath, geo: geoip.Open(logx.GeoIPPath(cfgPath)), log: log, discoverKick: make(chan struct{}, 1), probes: map[string]probeResult{},
		lastDeploy: map[string]time.Time{}, rollingBack: map[string]bool{}, srvMetrics: servermetrics.NewMonitor()}
}

// SetDevMode marca entorno `rkt dev` (proxy Vite): relaja CSRF
// loopback<->loopback en la API. Solo el comando dev lo activa.
func (p *Program) SetDevMode(v bool) { p.devMode = v }

// gh devuelve el manager de GitHub App (atómico: se reemplaza al crear la App).
func (p *Program) gh() *github.Manager { return p.ghm.Load() }

// Start lanza Run en goroutine (contrato kardianos/service).
// isHealthPath separa el control por loopback de la SPA en la raíz.
func isHealthPath(p string) bool {
	if p == "/healthz" {
		return true
	}
	for _, prefix := range []string{"/restart/", "/build/", "/deploy/", "/rollback/", "/retry/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func (p *Program) Start(_ service.Service) error {
	go p.Run()
	return nil
}

// hasTLS refleja el listener 443 real (bindeado), no la config: con
// hot-reload la config adelanta al listener y redirigir entonces
// mandaría a un https que aún no existe.
func (p *Program) hasTLS() func() bool {
	return func() bool {
		p.stateMu.Lock()
		defer p.stateMu.Unlock()
		return p.srvTLS != nil
	}
}

// Stop apaga todos los servidores con timeout.
func (p *Program) Stop(_ service.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.stateMu.Lock()
	srvTLS, srvHTTP, srvHealth, srvDashboard := p.srvTLS, p.srvHTTP, p.srvHealth, p.srvDashboard
	sup, alog, db, cron := p.sup, p.alog, p.db, p.cron
	p.stateMu.Unlock()
	if srvTLS != nil {
		_ = srvTLS.Shutdown(ctx)
	}
	if srvHTTP != nil {
		_ = srvHTTP.Shutdown(ctx)
	}
	if srvHealth != nil {
		_ = srvHealth.Shutdown(ctx)
	}
	if srvDashboard != nil {
		_ = srvDashboard.Shutdown(ctx)
	}
	if sup != nil {
		sup.Stop()
	}
	if cron != nil {
		cron.Stop()
	}
	if alog != nil {
		_ = alog.Close()
	}
	if db != nil {
		_ = db.Close()
	}
	return nil
}

// Run carga infra + flota desde SQLite y sirve. Bloquea en TLS.
func (p *Program) Run() {
	defer func() {
		if v := recover(); v != nil {
			// En servicio no hay stderr visible: deja constancia y muere.
			p.log.Error("panic fatal", "v", v)
			os.Exit(1)
		}
	}()
	cfg, db, err := cfgfile.LoadMerged(p.cfgPath)
	if err != nil {
		p.log.Error("config inválida", "err", err)
		os.Exit(1)
	}
	// Certs bajo el home (antes: dir de plataforma). Solo cuando no hay
	// valor propio: instalaciones viejas con ruta explícita no se tocan.
	// En memoria (sin persistir): el default efectivo, no un setting.
	if cfg.ACMECacheDir == "" || cfg.ACMECacheDir == platform.DefaultCertsDir() {
		cfg.ACMECacheDir = platform.CertsDir(logx.HomeDir(p.cfgPath))
	}
	// Migrar apps con layout antiguo (Dir sin sufijo .a/.b) al layout de
	// slots del blue-green: renombra <name> → <name>.a y persiste el Dir.
	if migrated, merr := migrateSlots(db, cfg); merr != nil {
		p.log.Warn("migración de slots incompleta", "err", merr)
	} else if migrated != nil {
		cfg = migrated
	}
	// Cortes no confirmados (el servicio murió a mitad del swap): la DB
	// puede decir slot nuevo con proceso viejo. Se marcan aborted con
	// evento; el supervisor arranca por manifiesto y el operador decide.
	if db != nil {
		if pending, err := db.PendingCutovers(); err != nil {
			p.log.Warn("journal de cortes ilegible", "err", err)
		} else {
			for _, pc := range pending {
				_ = db.FinishCutover(pc.ID, "aborted")
				_ = db.InsertEvent("failed", pc.App, "corte interrumpido por reinicio (dir "+pc.ToDir+")")
				p.log.Warn("corte sin confirmar, marcado aborted", "app", pc.App, "dir", pc.ToDir)
			}
		}
	}
	// Puertos grabados ocupados por terceros (TOCTOU del pool): reubicar
	// antes de arrancar para no entrar en crashloop.
	if cp2, ok := reallocateBusyPorts(db, cfg.Apps, nil, p.log); ok {
		cfg = cp2
	}
	live := config.NewLive(cfg)
	sup := supervisor.New(cfg.Apps, cfg.GlobalEnv, filepath.Join(logx.DirFor(p.cfgPath), "apps"), p.log)
	cronRunner := cronjobs.New(live, db, logx.DirFor(p.cfgPath), p.log)
	p.stateMu.Lock()
	p.db = db
	p.live = live
	p.sup = sup
	p.cron = cronRunner
	p.hub = hub.New()
	p.stateMu.Unlock()

	if db != nil {
		sup.OnEvent = func(kind, app, msg string) {
			if err := db.InsertEvent(kind, app, msg); err != nil {
				p.log.Warn("evento no guardado", "err", err)
			}
			if kind == "failed" {
				p.maybeAutoRollback(app)
			}
		}
	}
	sup.Start()
	cronRunner.Start()

	apiH := api.NewHandler(api.Deps{DB: db, Live: live, Version: build.Version, DevMode: p.devMode, LogsDir: logx.DirFor(p.cfgPath), GeoIP: p.geo,
		GitResolve: func(repo string) (gitcred.Access, error) {
			return gitcred.Resolve(p.db, logx.SecretsDir(p.cfgPath), p.gh(), repo, "")
		},
		GithubRevalidate: p.githubRevalidate, Canary: canary.Open(logx.HomeDir(p.cfgPath)), HomeDir: logx.HomeDir(p.cfgPath), CfgPath: p.cfgPath, AppsBase: platform.AppsDir(logx.HomeDir(p.cfgPath)), Apps: p.appStatuses, Restart: p.restartApp, Apply: func(apps []config.App, global map[string]string) {
			running := map[string]bool{}
			for _, s := range p.sup.Snapshot() {
				if s.Running {
					running[s.Name] = true
				}
			}
			if cp, ok := reallocateBusyPorts(db, apps, running, p.log); ok {
				live.Swap(cp)
				apps = cp.Apps
			}
			p.sup.Apply(apps, global)
			if p.cron != nil {
				p.cron.Apply()
			}
		}, Build: p.buildLocal, Deploy: p.deployApp, DeployAsync: p.deployAppAsync, Rollback: p.rollbackApp, Retry: p.retryDeployment, Cancel: p.cancelDeploy, Server: p.srvMetrics, Cron: p.cron, TLS: p.tlsStatus, Github: p.githubStatus, GithubRepos: p.githubRepos, GithubDisconnect: p.disconnectGithub, GithubTest: p.testGithub, GithubManager: p.gh, Hub: p.hub, Pending: p.pendingRestartGet})
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		p.log.Error("dashboard sin dist", "err", err)
		sub = nilFS{}
	}
	dashH := dashboard.NewHandler(sub)
	mcpH := mcpAuth(db, mcpserver.New(p.cfgPath).HTTPHandler())
	// Freno anti-abuso en /mcp: un token filtrado no debe agotar el
	// servicio con long-polls encadenados. Generoso para no estorbar
	// a agentes legítimos (los polls largos ocupan, no golpean).
	mcpH = ratelimit.New(8, 32).Middleware(mcpH, p.log)
	gate := dashboardGate(live, apiH, dashH, mcpH, proxy.NewHandlerWithFailFast(live, p.log, p.failedTarget))
	handler := http.Handler(gate)
	var alog *accesslog.Logger
	if l, err := accesslog.New(logx.AccessPath(p.cfgPath)); err != nil {
		p.log.Warn("access log desactivado", "err", err)
	} else {
		alog = l
		p.stateMu.Lock()
		p.alog = alog
		p.stateMu.Unlock()
		handler = alog.Middleware(gate)
	}
	var challenge http.Handler
	var mgr *autocert.Manager
	var ca *devca.CA
	if cfg.LocalCA {
		ca, err = devca.Load(filepath.Join(logx.DirFor(p.cfgPath), "devca"))
		if err != nil {
			p.log.Error("CA local inválida", "err", err)
			os.Exit(1)
		}
		p.stateMu.Lock()
		p.devca = ca
		p.stateMu.Unlock()
		if err := ca.EnsureTrusted(); err != nil {
			p.log.Warn("CA local sin confiar: el navegador avisará; importa el PEM a mano", "ca", ca.CertPath(), "err", err)
		} else {
			p.log.Info("CA local confiada (Root del usuario)", "ca", ca.CertPath())
		}
	} else {
		mgr = acme.NewManager(live, p.log)
		p.stateMu.Lock()
		p.mgr = mgr
		p.stateMu.Unlock()
		challenge = mgr.HTTPHandler(nil) // eager: activa http-01 como fallback
	}
	customCerts, err := loadCustomCerts(cfg.TLSCerts)
	if err != nil {
		p.log.Error("tls_certs inválidos", "err", err)
		os.Exit(1)
	}
	errLog := slog.NewLogLogger(p.log.Handler(), slog.LevelError)
	var control http.Handler
	{
		mux := http.NewServeMux()
		mux.Handle("/_rkt/new-app", hook.NewAppFormHandler())
		if cfg.Github != nil && cfg.Github.WebhookSecret != "" {
			secret := cfg.Github.WebhookSecret
			mux.Handle("/_rkt/hook/github", hook.NewHandler(hook.Deps{
				Live: live, Secret: secret, Log: p.log,
				DeployFn: func(name, actor string) {
					go func() {
						if rev, err := p.deployApp(name, actor); err != nil {
							p.log.Error("deploy por hook fallido", "app", name, "err", err)
						} else {
							p.log.Info("deploy por hook ok", "app", name, "rev", rev)
						}
					}()
				},
				OnInstall: p.onInstallEvent,
			}))
		}
		mux.Handle("/_rkt/setup", hook.SetupHandler(hook.SetupDeps{
			Log:          p.log,
			Verify:       p.verifyInstallation,
			OnSetup:      p.onSetup,
			DashboardURL: dashboardReturnURL(cfg),
			Exchange: func(code string) (*github.ManifestCredentials, error) {
				return github.ExchangeManifestCode("", code)
			},
			OnManifest:   p.onManifest,
			ConsumeState: db.ConsumeSetupState,
		}))
		control = mux
	}
	srvHTTP := server.NewHTTP(cfg, live, p.hasTLS(), challenge, handler, control, errLog)
	var srvTLS *http.Server
	switch {
	case cfg.LocalCA:
		srvTLS = server.NewHTTPS(cfg, wrapCert(customCerts, ca.GetCertificate), handler, control, errLog)
		p.log.Info("TLS de desarrollo con CA local", "ca", ca.CertPath())
	case cfg.ListenHTTPS != "":
		srvTLS = server.NewHTTPS(cfg, wrapCert(customCerts, mgr.GetCertificate), handler, control, errLog)
	default:
		p.log.Warn("TLS desactivado (listen_https vacío): solo HTTP")
	}
	p.stateMu.Lock()
	p.srvHTTP, p.srvTLS = srvHTTP, srvTLS
	p.stateMu.Unlock()

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
			if p.srvTLS == nil {
				os.Exit(1)
			}
		}
	}()
	shareDashPort := false
	if cfg.Healthz.Enabled {
		controlToken, err := logx.EnsureControlToken(p.cfgPath)
		if err != nil {
			p.log.Error("token de control no disponible; acciones bloqueadas", "err", err)
		}
		healthH := health.NewHandler(health.Deps{Live: live, Version: build.Version, Apps: p.appStatuses, Restart: p.restartApp, Deploy: p.deployApp, Rollback: p.rollbackApp, Retry: p.retryDeployment, Build: p.buildLocal, Token: controlToken})
		shareDashPort = cfg.Dashboard.Listen != "" && portOf(cfg.Dashboard.Listen) == portOf(cfg.Healthz.Listen)
		addr := cfg.Healthz.Listen
		if shareDashPort {
			// Windows no deja bindear 0.0.0.0:8090 y 127.0.0.1:8090 a la
			// vez. Un solo listener sirve consola pública y control local.
			addr = cfg.Dashboard.Listen
		}
		srvHealth := &http.Server{
			Addr: addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/mcp" {
					mcpH.ServeHTTP(w, r)
					return
				}
				if shareDashPort && !isLoopbackRemote(r) {
					if strings.HasPrefix(r.URL.Path, "/api/") {
						apiH.ServeHTTP(w, r)
						return
					}
					if r.URL.Path == "/metrics" || isHealthPath(r.URL.Path) {
						http.NotFound(w, r)
						return
					}
					dashH.ServeHTTP(w, r)
					return
				}
				// Métricas Prometheus solo en loopback.
				if r.URL.Path == "/metrics" {
					w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
					_, _ = io.WriteString(w, p.metricsText())
					return
				}
				// API primero: nada del SPA puede tapar /api/v1.
				if strings.HasPrefix(r.URL.Path, "/api/") {
					apiH.ServeHTTP(w, r)
					return
				}
				// Endpoints de control (healthz y acciones por loopback).
				if isHealthPath(r.URL.Path) {
					healthH.ServeHTTP(w, r)
					return
				}
				// Enlaces viejos con /dashboard: una sola redirección.
				if strings.HasPrefix(r.URL.Path, "/dashboard/") {
					target := strings.TrimPrefix(r.URL.Path, "/dashboard")
					if target == "" {
						target = "/"
					}
					if r.URL.RawQuery != "" {
						target += "?" + r.URL.RawQuery
					}
					http.Redirect(w, r, target, http.StatusMovedPermanently)
					return
				}
				dashH.ServeHTTP(w, r)
			}),
			// Sin WriteTimeout: POST /deploy/* tarda minutos (solo loopback).
			ReadTimeout: 10 * time.Second,
		}
		p.stateMu.Lock()
		p.srvHealth = srvHealth
		p.stateMu.Unlock()
		go func() {
			if err := srvHealth.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.log.Error("healthz muerto", "err", err)
			}
		}()
	}
	if cfg.Dashboard.Listen != "" && !shareDashPort {
		console := consoleOnly(apiH, dashH, mcpH)
		if alog != nil {
			console = alog.Middleware(console)
		}
		srvDashboard := &http.Server{
			Addr:              cfg.Dashboard.Listen,
			Handler:           console,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		p.stateMu.Lock()
		p.srvDashboard = srvDashboard
		p.stateMu.Unlock()
		serve := srvDashboard.ListenAndServe
		if cfg.Dashboard.TLS == "auto" {
			if tcfg, err := p.dashboardTLS(cfg, ca, mgr); err != nil {
				p.log.Error("dashboard TLS: sin certificado, sirvo HTTP plano", "err", err)
			} else {
				srvDashboard.TLSConfig = tcfg
				serve = func() error { return srvDashboard.ListenAndServeTLS("", "") }
			}
		}
		if srvDashboard.TLSConfig == nil {
			p.log.Warn("dashboard expuesto sin TLS en listener propio; activa dashboard_tls=auto o usa dashboard_host en 443",
				"listen", cfg.Dashboard.Listen)
		}
		go func() {
			if err := serve(); err != nil && err != http.ErrServerClosed {
				p.log.Error("dashboard muerto", "err", err)
			}
		}()
	}
	if cfg.Dashboard.Host != "" {
		p.log.Info("dashboard público", "host", cfg.Dashboard.Host)
	}
	p.log.Info("hot-reload activo", "poll", pollInterval.String())
	go p.watchMerged(live, db)
	go p.fleetLoop(live, db)
	go p.alertLoop(live, db)
	go p.maintenanceLoop(live, db)
	go p.attackLoop(db)
	if cfg.Github != nil {
		gh, err := github.New(cfg.Github.AppID, cfg.Github.KeyFile, "")
		if err != nil {
			p.log.Error("github app inválida, sin auto-deploy", "err", err)
		} else {
			p.ghm.Store(gh)
			p.log.Info("auto-deploy activo", "poll", cfg.Github.Poll().String())
			if p.autoStarted.CompareAndSwap(false, true) {
				go p.autoDeploy(live)
			}
		}
	}
	if p.srvTLS == nil {
		select {} // sin TLS el HTTP manda; bloquea aquí
	}
	if err := p.srvTLS.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		p.log.Error("https muerto", "err", err)
		os.Exit(1)
	}
}

// failedTarget dice si un backend es de una app en failed terminal: el
// supervisor ya no la reintenta, así que el proxy va directo a 502 sin
// gastar restart_grace. Solo failed (definitivo); en arranque o crashloop
// el grace sigue cubriendo los reintentos.
func (p *Program) failedTarget(target string) bool {
	if p.sup == nil {
		return false
	}
	app := p.live.Get().AppForTarget(target)
	if app == nil {
		return false
	}
	for _, s := range p.sup.Snapshot() {
		if s.Name == app.Name {
			return s.Failed
		}
	}
	return false
}

// appStatuses adapta supervisor a health (nil si no hay manager).
func (p *Program) appStatuses() []health.AppStatus {
	if p.sup == nil {
		return nil
	}
	out := []health.AppStatus{}
	live := p.live.Get()
	for _, s := range p.sup.Snapshot() {
		ok := true
		if s.Running {
			ok = p.probeApp(s.Name, live)
		}
		out = append(out, health.AppStatus{
			Name: s.Name, Running: s.Running, Pid: s.Pid,
			Restarts: s.Restarts, Failed: s.Failed, HTTPOK: ok,
		})
	}
	// Externas: no hay supervisor; el estado es si el dev server escucha.
	for i := range live.Apps {
		a := &live.Apps[i]
		if !a.External() {
			continue
		}
		up := portOpen(a.Env["PORT"])
		out = append(out, health.AppStatus{
			Name: a.Name, Running: up, HTTPOK: up && p.probeApp(a.Name, live),
		})
	}
	return out
}

// portOpen dial corto a 127.0.0.1:<port> (estado de app externa).
func portOpen(port string) bool {
	if port == "" {
		return false
	}
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// probeApp verifica HTTP al puerto de la app (cache 3s: evita martillar
// con el polling del dashboard). Sin puerto no hay probe.
func (p *Program) probeApp(name string, live *config.Config) bool {
	var port, path string
	for i := range live.Apps {
		if live.Apps[i].Name == name {
			port = live.Apps[i].Env["PORT"]
			path = live.Apps[i].HealthPath
		}
	}
	if port == "" {
		return true
	}
	if path == "" {
		path = "/"
	}
	p.probeMu.Lock()
	if v, ok := p.probes[name]; ok && time.Since(v.at) < 3*time.Second {
		p.probeMu.Unlock()
		return v.ok
	}
	p.probeMu.Unlock()

	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + port + path)
	ok := false
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		_ = resp.Body.Close()
		ok = resp.StatusCode < 500
	}
	p.probeMu.Lock()
	p.probes[name] = probeResult{at: time.Now(), ok: ok}
	p.probeMu.Unlock()
	return ok
}

// restartApp reinicia una app gestionada; externas no tienen proceso.
func (p *Program) restartApp(name string) bool {
	if a := cfgfile.FindApp(p.live.Get(), name); a != nil && a.External() {
		return false
	}
	running := map[string]bool{}
	if p.sup != nil {
		for _, s := range p.sup.Snapshot() {
			if s.Running {
				running[s.Name] = true
			}
		}
	}
	if cp, ok := reallocateBusyPorts(p.db, p.live.Get().Apps, running, p.log); ok {
		p.live.Swap(cp)
	}
	return p.sup.Restart(name)
}

// supCtl adapta Manager a deploy.Controller.
type supCtl struct {
	sup  *supervisor.Manager
	db   *store.DB
	live *config.Live
}

func (c *supCtl) SwapAndRestart(name string, swap func() error) error {
	return c.sup.SwapAndRestart(name, swap)
}

func (c *supCtl) Snapshot() []supervisor.Status {
	return c.sup.Snapshot()
}

func (c *supCtl) StartCanary(name, dir, port string, out io.Writer) (*exec.Cmd, error) {
	return c.sup.StartCanary(name, dir, port, out)
}

// Cutover persiste el corte (Dir, PORT y ruta→port) y adopta el canario.
// La ruta cambia en el próximo request vía Live.Swap; el canario ya sano.
// Journal primero: el intent queda 'started' y se confirma tras adoptar;
// si el servicio muere a mitad, el arranque lo marca aborted (ver Run).
func (c *supCtl) Cutover(name string, cmd *exec.Cmd, dir, port string) error {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("puerto canario inválido %q: %w", port, err)
	}
	var fromDir string
	var oldDir, oldStatic string
	var oldArgs []string
	if a := cfgfile.FindApp(c.live.Get(), name); a != nil {
		fromDir = a.Dir
		oldDir, oldStatic, oldArgs = a.Dir, a.Static, a.Args
	}
	var cutID int64
	if c.db != nil {
		if id, ierr := c.db.InsertCutover(name, fromDir, dir, port, "", ""); ierr == nil {
			cutID = id
		}
	}
	// Rebase estático calculado fuera del MutateFleet para reusarlo en el
	// patch Y en SwapLive (ver abajo).
	var newStatic string
	var newArgs []string
	finish := func(status string) {
		if c.db != nil && cutID > 0 {
			_ = c.db.FinishCutover(cutID, status)
		}
	}
	cp, err := cfgfile.MutateFleet(c.db, func(cp *config.Config) error {
		// El path estático guardado apunta al slot que era live: rebasarlo
		// al que pasa a serlo o el próximo canario serviría el slot viejo.
		// Se calculan fuera para reusarlos en SwapLive: si el loop conserva
		// el static/args viejos, el Apply de fondo lo reemplazaría y el
		// puerto volvería a estar ocupado → crashloop.
		p := cfgfile.AppPatch{Dir: &dir, Port: &portInt}
		if ns, ok := supervisor.RebaseStatic(oldDir, oldStatic, dir); ok {
			newStatic = ns
			p.Static = &ns
		}
		if len(oldArgs) >= 2 && oldArgs[0] == "__static" {
			if na, ok := supervisor.RebaseStatic(oldDir, oldArgs[1], dir); ok {
				args := append([]string{oldArgs[0], na}, oldArgs[2:]...)
				newArgs = args
				p.Args = &args
			}
		}
		return cfgfile.UpdateApp(cp, name, p)
	})
	if err != nil {
		finish("failed")
		return err
	}
	c.live.Swap(cp)
	if err := c.sup.SwapLive(name, cmd, dir, port, newStatic, newArgs); err != nil {
		finish("failed")
		return err
	}
	finish("done")
	return nil
}

// migrateSlots convierte apps con layout antiguo (Dir sin sufijo .a/.b) al
// layout de slots del blue-green: renombra <name> → <name>.a y persiste el
// nuevo Dir. Devuelve la config migrada (nil si no hubo cambios).
func migrateSlots(db *store.DB, cfg *config.Config) (*config.Config, error) {
	type mv struct{ name, from, to string }
	var moves []mv
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if a.External() || a.Dir == "" ||
			strings.HasSuffix(a.Dir, ".a") || strings.HasSuffix(a.Dir, ".b") {
			continue
		}
		moves = append(moves, mv{a.Name, a.Dir, a.Dir + ".a"})
	}
	if len(moves) == 0 {
		return nil, nil
	}
	for _, m := range moves {
		if st, err := os.Stat(m.from); err == nil && st.IsDir() {
			if err := os.Rename(m.from, m.to); err != nil {
				return nil, fmt.Errorf("migrar slot %s: %w", m.name, err)
			}
		}
	}
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		for i := range cp.Apps {
			a := &cp.Apps[i]
			if a.External() || a.Dir == "" ||
				strings.HasSuffix(a.Dir, ".a") || strings.HasSuffix(a.Dir, ".b") {
				continue
			}
			from, to := a.Dir, a.Dir+".a"
			a.Dir = to
			// El root estático y los args __static apuntaban al dir viejo:
			// si no se reescriben, el supervisor sirve una ruta que ya no
			// existe ("compila primero" eterno).
			rebased := func(p string) string {
				if p == from {
					return to
				}
				if strings.HasPrefix(p, from+"\\") || strings.HasPrefix(p, from+"/") {
					return to + p[len(from):]
				}
				return p
			}
			a.Static = rebased(a.Static)
			for j, arg := range a.Args {
				a.Args[j] = rebased(arg)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cp, nil
}

// pendingRestartGet campos que exigen restart (para /api/v1/status).
func (p *Program) pendingRestartGet() []string {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	return append([]string{}, p.pendingRestart...)
}

func (p *Program) setPendingRestart(fields []string) {
	p.pendingMu.Lock()
	p.pendingRestart = append([]string{}, fields...)
	p.pendingMu.Unlock()
}

// nilFS FS vacío (dist ausente): todo Stat falla -> dashboard 503.
type nilFS struct{}

func (nilFS) Open(name string) (fs.File, error) { return nil, fs.ErrNotExist }

// deployApp ejecuta un deploy en el servicio: token del entorno del
// SERVICIO (variable de máquina para SYSTEM), log a archivo.
// actor queda en historial (usuario api, hook, discover, auto, local).
func (p *Program) deployApp(name, actor string) (string, error) {
	p.normalizeNextStart(name)
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
	return p.runDeploy(p.runtimeApp(*app, cfg), actor)
}

// runtimeApp copia la app con su entorno efectivo (globales pisadas por las
// propias) para builds/deploys. El manifiesto guardado no se muta.
func (p *Program) runtimeApp(app config.App, cfg *config.Config) config.App {
	app.Env = cfg.RuntimeEnv(app.Env)
	return app
}

// retryDeployment re-corre el rev exacto de un deploy previo
// (override de ref al SHA; hereda env actual + actor).
func (p *Program) retryDeployment(id int64, actor string) (string, error) {
	deps, err := p.db.ListDeployments("", 500)
	if err != nil {
		return "", err
	}
	var rev, appName string
	for _, d := range deps {
		if d.ID == id {
			rev, appName = d.Rev, d.App
		}
	}
	if rev == "" || appName == "" {
		return "", fmt.Errorf("deploy #%d sin rev repetible", id)
	}
	cfg := p.live.Get()
	for i := range cfg.Apps {
		if cfg.Apps[i].Name != appName {
			continue
		}
		app := cfg.Apps[i]
		if app.Deploy == nil {
			return "", fmt.Errorf("app %q sin bloque deploy", appName)
		}
		app.Deploy.Ref = rev
		return p.runDeploy(p.runtimeApp(app, cfg), actor+" (retry #"+strconv.FormatInt(id, 10)+")")
	}
	return "", fmt.Errorf("app %q inexistente", appName)
}

// isStandardPnpmStart detecta el arranque por defecto de una app Node.
func isStandardPnpmStart(a *config.App) bool {
	base := strings.ToLower(filepath.Base(a.Command))
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".cmd"), ".exe")
	return base == "pnpm" && len(a.Args) == 2 && a.Args[0] == "run" && a.Args[1] == "start"
}

// buildHintForLog traduce firmas conocidas del log de build a pasos
// accionables en español. Devuelve "" si no reconoce nada.
func buildHintForLog(logPath string) string {
	tail := readTail(logPath, 16<<10)
	switch {
	case strings.Contains(tail, "ERR_PNPM_IGNORED_BUILDS") || strings.Contains(tail, "approve-builds"):
		return "pnpm bloqueó scripts de build por seguridad: en el proyecto corre `pnpm approve-builds` (elige los de confianza) y recompila."
	case strings.Contains(tail, "EPERM") && strings.Contains(tail, "symlink"):
		return "Windows bloqueó un symlink del build (Next standalone): activa el modo desarrollador (Ajustes > Para desarrolladores) o compila como admin."
	default:
		return ""
	}
}

// readTail lee hasta n bytes finales del archivo ("" si no se puede).
func readTail(path string, n int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	off := st.Size() - n
	if off < 0 {
		off = 0
	}
	buf := make([]byte, st.Size()-off)
	_, _ = f.ReadAt(buf, off)
	return string(buf)
}

// normalizeNextStart migra apps Next creadas antes del arranque canónico:
// `pnpm run start` puede fijar -p 3001 y desincronizar el proxy. Deja
// `pnpm exec next start -H 127.0.0.1` para que use PORT.
func (p *Program) normalizeNextStart(name string) {
	if p.db == nil || p.sup == nil {
		return
	}
	cfg := p.live.Get()
	app := cfgfile.FindApp(cfg, name)
	if app == nil || app.External() || !isStandardPnpmStart(app) {
		return
	}
	plan, err := buildpack.DetectDir(app.Dir)
	if err != nil || plan.Kind != "next" || len(plan.Start) == 0 {
		return
	}
	cmd := plan.Start[0]
	args := append([]string(nil), plan.Start[1:]...)
	cp, err := cfgfile.MutateFleet(p.db, func(cp *config.Config) error {
		return cfgfile.UpdateApp(cp, name, cfgfile.AppPatch{Command: &cmd, Args: &args})
	})
	if err != nil {
		p.log.Warn("normalizar start de Next", "app", name, "err", err)
		return
	}
	p.live.Swap(cp)
	p.sup.Apply(cp.Apps, cp.GlobalEnv)
	p.log.Info("start de Next normalizado (PORT + loopback)", "app", name, "start", strings.Join(plan.Start, " "))
}

// buildLocal compila una app de directorio local con su plan (buildpack):
// registra la salida como deployment en curso, corre install+build y
// reinicia la unidad al terminar. Devuelve el id para seguir el log.
func (p *Program) buildLocal(name, actor string) (int64, error) {
	if err := p.beginOp(name); err != nil {
		return 0, err
	}
	p.normalizeNextStart(name)
	release := true
	defer func() {
		if release {
			p.endOp(name)
		}
	}()
	cfg := p.live.Get()
	// Reconciliación inmediata: una app recién creada puede no estar aún en
	// el supervisor (el watcher tarda 10s) y el build fallaría "inexistente".
	if p.sup != nil {
		p.sup.Apply(cfg.Apps, cfg.GlobalEnv)
	}
	var app *config.App
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == name {
			app = &cfg.Apps[i]
		}
	}
	if app == nil {
		return 0, fmt.Errorf("app %q inexistente", name)
	}
	if app.External() {
		return 0, fmt.Errorf("app %q es externa: no tiene build; corre el dev server en %s", name, app.Dir)
	}
	plan, err := buildpack.DetectDir(app.Dir)
	if err != nil {
		return 0, err
	}
	steps := append(append([]string{}, plan.Install...), plan.Build...)
	if len(steps) == 0 {
		if plan.IsStatic() {
			return 0, fmt.Errorf("la app ya sirve su carpeta estática (%s): edita los archivos y recarga; no hay nada que compilar", plan.Static(app.Dir))
		}
		return 0, fmt.Errorf("el plan %q no define pasos de build", plan.Kind)
	}
	logs := logx.DirFor(p.cfgPath)
	if err := os.MkdirAll(filepath.Join(logs, "deploys"), 0o755); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(
		filepath.Join(logs, "deploys", app.Name+"-build-"+time.Now().Format("20060102-150405")+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	var depID int64
	target := p.runtimeApp(*app, cfg)
	if p.db != nil {
		depID, _ = p.db.InsertDeployment(app.Name, "local", out.Name(), actor)
	}
	release = false
	go func() {
		defer p.endOp(target.Name)
		defer out.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		// La app local se compila en su propio directorio: hay que pararla
		// antes. Si no, pnpm no puede reemplazar node_modules/.pnpm porque
		// el proceso vivo tiene handles abiertos (Windows: acceso denegado).
		build := func() error {
			return deploy.RunStepsContext(ctx, steps, target.Dir, target.Env, out)
		}
		var err error
		if p.sup != nil {
			err = p.sup.SwapAndRestart(target.Name, build)
		} else {
			err = build()
		}
		if err == nil && p.sup != nil {
			// el build no basta: la unidad debe responder HTTP tras reiniciar
			if perr := deploy.WaitAppHTTP(target, 10*time.Second); perr != nil {
				err = fmt.Errorf("build ok pero la app no responde: %w", perr)
			}
		}
		status := "ok"
		if err != nil {
			status = "failed"
			fmt.Fprintf(out, "\n✗ build: %v\n", err)
			// Firmas conocidas con arreglo documentado: el log crudo
			// (pnpm en inglés, webpack en inglés) no le dice al operador
			// qué hacer; esta línea sí.
			if hint := buildHintForLog(out.Name()); hint != "" {
				fmt.Fprintf(out, "\n! %s\n", hint)
			}
			p.log.Error("build local fallido", "app", target.Name, "err", err)
		}
		if p.db != nil && depID > 0 {
			if ferr := p.db.FinishDeployment(depID, status, "", "local"); ferr != nil {
				p.log.Warn("historial no actualizado", "err", ferr)
			}
		}
	}()
	return depID, nil
}

// runDeploy pipeline común deploy/retry (único hogar, síncrono).
func (p *Program) runDeploy(app config.App, actor string) (string, error) {
	_, rev, err := p.startDeploy(app, actor, false)
	return rev, err
}

// cancelDeploy cancela un despliegue en curso por id.
func (p *Program) cancelDeploy(id int64) bool {
	p.cancelMu.Lock()
	cancel, ok := p.deployCancels[id]
	p.cancelMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// deployAppAsync arranca el deploy en segundo plano y devuelve el id de la
// salida para seguirlo en vivo (creación de apps estilo Vercel).
func (p *Program) deployAppAsync(name, actor string) (int64, error) {
	p.normalizeNextStart(name)
	cfg := p.live.Get()
	var app *config.App
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == name {
			app = &cfg.Apps[i]
		}
	}
	if app == nil {
		return 0, fmt.Errorf("app %q inexistente", name)
	}
	if app.Deploy == nil {
		return 0, fmt.Errorf("app %q sin bloque deploy", name)
	}
	id, _, err := p.startDeploy(p.runtimeApp(*app, cfg), actor, true)
	return id, err
}

// startDeploy prepara log + registro y ejecuta (async o no) el pipeline.
func (p *Program) startDeploy(app config.App, actor string, async bool) (int64, string, error) {
	if app.External() {
		return 0, "", fmt.Errorf("app %q es externa: no tiene pipeline; corre el dev server en %s", app.Name, app.Dir)
	}
	if p.sup != nil {
		p.sup.Apply(p.live.Get().Apps, p.live.Get().GlobalEnv)
	}
	if err := p.beginOp(app.Name); err != nil {
		return 0, "", err
	}
	release := true
	defer func() {
		if release {
			p.endOp(app.Name)
		}
	}()
	acc, err := p.gitAccess(&app)
	if err != nil {
		return 0, "", err
	}
	token := acc.Token
	dir := logx.DirFor(p.cfgPath)
	if err := os.MkdirAll(filepath.Join(dir, "deploys"), 0o755); err != nil {
		return 0, "", err
	}
	out, err := os.OpenFile(
		filepath.Join(dir, "deploys", app.Name+"-"+time.Now().Format("20060102-150405")+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, "", err
	}
	var depID int64
	prevRev := deploy.HeadRev(app.Dir)
	if p.db != nil {
		depID, _ = p.db.InsertDeployment(app.Name, "", out.Name(), actor)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if acc.SSHKey != "" {
		ctx = deploy.WithSSHKey(ctx, acc.SSHKey)
	}
	if depID > 0 {
		p.cancelMu.Lock()
		if p.deployCancels == nil {
			p.deployCancels = map[int64]context.CancelFunc{}
		}
		p.deployCancels[depID] = cancel
		p.cancelMu.Unlock()
	}
	dropCancel := func() {
		cancel()
		if depID > 0 {
			p.cancelMu.Lock()
			delete(p.deployCancels, depID)
			p.cancelMu.Unlock()
		}
	}
	finish := func(rev string, runErr error) {
		if p.db != nil && depID > 0 {
			status := "ok"
			if runErr != nil {
				status = "failed"
				if errors.Is(runErr, context.Canceled) {
					status = "cancelled"
				}
			}
			if ferr := p.db.FinishDeployment(depID, status, prevRev, rev); ferr != nil {
				p.log.Warn("historial no actualizado", "err", ferr)
			}
		}
	}
	if async {
		release = false
		go func() {
			defer dropCancel()
			defer p.endOp(app.Name)
			defer out.Close()
			rev, runErr := deploy.RunContext(ctx, app, token, &supCtl{sup: p.sup, db: p.db, live: p.live}, out, p.log)
			if runErr != nil {
				fmt.Fprintf(out, "\n✗ deploy fallido: %v\n", runErr)
			} else {
				p.markDeployed(app.Name)
			}
			finish(rev, runErr)
		}()
		return depID, "", nil
	}
	defer out.Close()
	defer dropCancel()
	rev, err := deploy.RunContext(ctx, app, token, &supCtl{sup: p.sup, db: p.db, live: p.live}, out, p.log)
	if err != nil {
		fmt.Fprintf(out, "\n✗ deploy fallido: %v\n", err)
	} else {
		p.markDeployed(app.Name)
	}
	finish(rev, err)
	if err != nil {
		return depID, "", err
	}
	return depID, rev, nil
}

// gitAccess resuelve el acceso al repo con el orden único de
// gitcred.Resolve (GitHub App primero).
func (p *Program) gitAccess(app *config.App) (gitcred.Access, error) {
	acc, err := gitcred.Resolve(p.db, logx.SecretsDir(p.cfgPath), p.gh(), app.Deploy.Repo, app.Deploy.TokenEnv)
	if err == nil && acc.Note != "" {
		p.log.Info("git: acceso por mecanismo alterno", "app", app.Name, "via", acc.Via, "nota", acc.Note)
	}
	return acc, err
}

// tlsStatus interroga la cache ACME (existe/expira). Sin manager/TLS: off.
func (p *Program) tlsStatus(host string) (bool, string) {
	if p.devca != nil {
		return true, ""
	}
	if p.mgr == nil {
		return false, ""
	}
	// wildcard *.ejemplo.com vive bajo base sin *.
	h := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "*.")
	data, err := p.mgr.Cache.Get(context.Background(), h)
	if err != nil || len(data) == 0 {
		return false, ""
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return true, ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true, ""
	}
	return true, cert.NotAfter.UTC().Format("2006-01-02")
}

// markDeployed registra un deploy exitoso para el auto-rollback si la app
// entra en crashloop poco después (build que compila pero no arranca).
func (p *Program) markDeployed(name string) {
	p.deployMu.Lock()
	p.lastDeploy[name] = time.Now()
	p.deployMu.Unlock()
}

// maybeAutoRollback devuelve live.prev si la app falló dentro de la
// ventana post-deploy. Reintenta porque el deploy puede seguir soltando
// el lock de operación.
func (p *Program) maybeAutoRollback(name string) {
	p.deployMu.Lock()
	t, ok := p.lastDeploy[name]
	if !ok || time.Since(t) > 10*time.Minute || p.rollingBack[name] {
		p.deployMu.Unlock()
		return
	}
	delete(p.lastDeploy, name)
	p.rollingBack[name] = true
	p.deployMu.Unlock()
	go func() {
		defer func() {
			p.deployMu.Lock()
			delete(p.rollingBack, name)
			p.deployMu.Unlock()
		}()
		p.log.Warn("app falló tras deploy; rollback automático", "app", name)
		var err error
		for i := 0; i < 6; i++ {
			if err = p.rollbackApp(name, "auto-rollback"); err == nil {
				p.log.Info("rollback automático ok", "app", name)
				return
			}
			time.Sleep(5 * time.Second)
		}
		p.log.Error("rollback automático fallido", "app", name, "err", err)
	}()
}

// rollbackApp devuelve live.prev a live (rollback manual).
// También deja fila en historial (hereda env actual + actor).
func (p *Program) rollbackApp(name, actor string) error {
	if err := p.beginOp(name); err != nil {
		return err
	}
	defer p.endOp(name)
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
	if app.External() {
		return fmt.Errorf("app %q es externa: no tiene rollback; corre el dev server en %s", name, app.Dir)
	}
	var depID int64
	if p.db != nil {
		depID, _ = p.db.InsertDeployment(app.Name, "", "", actor)
	}
	err := deploy.Rollback(*app, &supCtl{sup: p.sup, db: p.db, live: p.live}, p.log)
	if p.db != nil && depID > 0 {
		status, rev := "ok", deploy.HeadRev(app.Dir)
		if err != nil {
			status, rev = "failed", ""
		}
		if ferr := p.db.FinishDeployment(depID, status, "", rev); ferr != nil {
			p.log.Warn("historial no actualizado", "err", ferr)
		}
	}
	return err
}

// alertLoop evalúa reglas cada minuto: failed por app desde events,
// error_rate global desde access.log (entries no traen app).
// Disparo = evento kind=alert + cooldown de ventana.
