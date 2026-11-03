// Package app orquesta el binario: carga config, arma handler,
// ACME y servidores. Implementa service.Interface para Windows.
package app

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/accesslog"
	"rkt-proxy/internal/acme"
	"rkt-proxy/internal/alerts"
	"rkt-proxy/internal/api"
	"rkt-proxy/internal/build"
	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dashboard"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/devca"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/hook"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/server"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/supervisor"
	"rkt-proxy/internal/weblog"
	"rkt-proxy/web"
)

// Version vive en internal/build (inyectada por CI). Ver build.Version.

// pollInterval revisa el archivo de config (mtime+tamaño).
// Poll y no fsnotify: portable, sin deps, suficiente a 10s.
const pollInterval = 10 * time.Second

// Program es el servicio: guarda servidores para Stop limpio.
type Program struct {
	cfgPath      string
	log          *slog.Logger
	live         *config.Live
	ghm          atomic.Pointer[github.Manager]
	autoStarted  atomic.Bool
	db           *store.DB
	discoverKick chan struct{}
	srvHTTP      *http.Server
	srvTLS       *http.Server
	srvHealth    *http.Server
	alog         *accesslog.Logger
	sup          *supervisor.Manager
	mgr          *autocert.Manager
	devca        *devca.CA
	probeMu      sync.Mutex
	probes       map[string]probeResult
	// ops serializa operaciones destructivas por app (deploy/build/rollback).
	opMu sync.Mutex
	ops  map[string]bool
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
	return &Program{cfgPath: cfgPath, log: log, discoverKick: make(chan struct{}, 1), probes: map[string]probeResult{}}
}

// gh devuelve el manager de GitHub App (atómico: se reemplaza al crear la App).
func (p *Program) gh() *github.Manager { return p.ghm.Load() }

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

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
	p.db = db
	live := config.NewLive(cfg)
	p.live = live

	p.sup = supervisor.New(cfg.Apps, cfg.GlobalEnv, filepath.Join(logx.DirFor(p.cfgPath), "apps"), p.log)
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
	p.mgr = nil
	var challenge http.Handler
	if cfg.LocalCA {
		ca, err := devca.Load(filepath.Join(logx.DirFor(p.cfgPath), "devca"))
		if err != nil {
			p.log.Error("CA local inválida", "err", err)
			os.Exit(1)
		}
		p.devca = ca
		if err := ca.EnsureTrusted(); err != nil {
			p.log.Warn("CA local sin confiar: el navegador avisará; importa el PEM a mano", "ca", ca.CertPath(), "err", err)
		} else {
			p.log.Info("CA local confiada (Root del usuario)", "ca", ca.CertPath())
		}
	} else {
		p.mgr = acme.NewManager(live, p.log)
		challenge = p.mgr.HTTPHandler(nil) // eager: activa http-01 como fallback
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
			Log:     p.log,
			Verify:  p.verifyInstallation,
			OnSetup: p.onSetup,
			Exchange: func(code string) (*github.ManifestCredentials, error) {
				return github.ExchangeManifestCode("", code)
			},
			OnManifest: p.onManifest,
		}))
		control = mux
	}
	p.srvHTTP = server.NewHTTP(cfg, live, challenge, handler, control, errLog)
	switch {
	case cfg.LocalCA:
		p.srvTLS = server.NewHTTPS(cfg, p.devca.GetCertificate, handler, control, errLog)
		p.log.Info("TLS de desarrollo con CA local", "ca", p.devca.CertPath())
	case cfg.ListenHTTPS != "":
		p.srvTLS = server.NewHTTPS(cfg, p.mgr.GetCertificate, handler, control, errLog)
	default:
		p.log.Warn("TLS desactivado (listen_https vacío): solo HTTP")
	}

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
	if cfg.Healthz.Enabled {
		healthH := health.NewHandler(health.Deps{Live: live, Version: build.Version, Apps: p.appStatuses, Restart: p.sup.Restart, Deploy: p.deployApp, Rollback: p.rollbackApp, Retry: p.retryDeployment, Build: p.buildLocal})
		apiH := api.NewHandler(api.Deps{DB: db, Live: live, Version: build.Version, LogsDir: logx.DirFor(p.cfgPath), Apps: p.appStatuses, Restart: p.sup.Restart, Apply: func(apps []config.App, global map[string]string) { p.sup.Apply(apps, global) }, Build: p.buildLocal, Deploy: p.deployApp, DeployAsync: p.deployAppAsync, Rollback: p.rollbackApp, Retry: p.retryDeployment, TLS: p.tlsStatus, Github: p.githubStatus, GithubRepos: p.githubRepos})
		sub, err := fs.Sub(web.Dist, "dist")
		if err != nil {
			p.log.Error("dashboard sin dist", "err", err)
			sub = nilFS{}
		}
		dashH := dashboard.NewHandler(sub)
		p.srvHealth = &http.Server{
			Addr: cfg.Healthz.Listen,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		go func() {
			if err := p.srvHealth.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				p.log.Error("healthz muerto", "err", err)
			}
		}()
	}
	p.log.Info("hot-reload activo", "poll", pollInterval.String())
	go p.watchMerged(live, db)
	go p.alertLoop(live, db)
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

// verifyInstallation confirma una instalación contra la API (setup browser).
func (p *Program) verifyInstallation(id int64) (string, error) {
	if p.gh() == nil {
		return "", fmt.Errorf("sin github app")
	}
	return p.gh().Installation(id)
}

// onSetup atiende el setup del browser tras instalar: verifica y dispara
// discovery (la API es la fuente; nada que persistir).
func (p *Program) onSetup(id int64, account string) error {
	p.log.Info("setup: instalación confirmada", "account", account, "installation", id)
	if p.gh() != nil {
		info, err := p.gh().InstallationDetail(id)
		if err != nil {
			p.log.Warn("setup: no pude leer la instalación", "err", err)
		} else {
			if info.Account != "" {
				account = info.Account
			}
			settings := map[string]string{
				"github_install_id":     strconv.FormatInt(id, 10),
				"github_account":        account,
				"github_repo_selection": info.RepositorySelection,
				"github_install_url":    info.HTMLURL,
				"github_install_target": info.TargetType,
			}
			for k, v := range settings {
				if err := p.db.SetSetting(k, v); err != nil {
					return err
				}
			}
			if tok, err := p.gh().TokenFor(id); err == nil {
				if repos, err := p.gh().Repositories(tok); err == nil {
					_ = p.db.SetSetting("github_repo_count", strconv.Itoa(len(repos)))
				}
			}
			if info.RepositorySelection != "all" {
				p.log.Warn("github: instalación con repos seleccionados; activa All repositories para clonar privados",
					"account", account, "settings", info.HTMLURL)
			}
		}
	}
	p.kickDiscovery()
	return nil
}

// onInstallEvent atiende webhooks installation* (alta/baja): la API es la
// fuente de verdad, aquí solo se dispara discovery inmediato.
func (p *Program) onInstallEvent(id int64, account, action string) {
	p.log.Info("hook: evento instalación", "action", action, "account", account)
	p.kickDiscovery()
}

// kickDiscovery pide pasada inmediata (no bloquea).
func (p *Program) kickDiscovery() {
	select {
	case p.discoverKick <- struct{}{}:
	default:
	}
}

// onManifest guarda credenciales del manifest (PEM + settings) y devuelve
// la URL de instalación para el browser.
func (p *Program) onManifest(c *github.ManifestCredentials) (string, error) {
	if p.db == nil {
		return "", fmt.Errorf("sin DB")
	}
	keyPath := logx.AppKeyPath(p.cfgPath)
	if err := os.WriteFile(keyPath, []byte(c.PEM), 0o600); err != nil {
		return "", fmt.Errorf("guardar key: %w", err)
	}
	for k, v := range map[string]string{
		"github_app_id":         strconv.FormatInt(c.ID, 10),
		"github_key_file":       keyPath,
		"github_webhook_secret": c.WebhookSecret,
	} {
		if err := p.db.SetSetting(k, v); err != nil {
			return "", err
		}
	}
	slug := c.Slug
	if slug == "" {
		slug = "rkt-deployer"
	}
	_ = p.db.SetSetting("github_app_slug", slug)
	if gh, err := github.New(c.ID, keyPath, ""); err != nil {
		p.log.Warn("github app creada pero manager inválido", "err", err)
	} else {
		p.ghm.Store(gh)
		if p.autoStarted.CompareAndSwap(false, true) {
			go p.autoDeploy(p.live)
		}
	}
	p.log.Info("app github creada, instala en browser", "slug", slug)
	return "/_rkt/setup?install=" + slug, nil
}

// setupOrigin devuelve el origen público para el manifest. Sin base_domain
// cae a loopback (GitHub permite crear Apps con URLs locales; sin webhooks).
func (p *Program) setupOrigin() (origin, domain string, local, tls bool) {
	base := strings.TrimSpace(p.db.GetSetting(store.KeyBaseDomain))
	if base != "" {
		return "https://" + base, base, false, true
	}
	cfg := p.live.Get()
	tls = cfg.ListenHTTPS != ""
	addr := cfg.ListenHTTP
	if tls {
		addr = cfg.ListenHTTPS
	}
	port := ""
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		port = addr[i+1:]
	}
	host := "localhost"
	if (tls && port != "" && port != "443") || (!tls && port != "" && port != "80") {
		host += ":" + port
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return scheme + "://" + host, host, true, tls
}

// githubStatus estado de la GitHub App para dashboard/API.
func (p *Program) githubStatus() (map[string]any, error) {
	out := map[string]any{"configured": false}
	origin, domain, local, tls := p.setupOrigin()
	label := "local"
	if !local {
		label = strings.ReplaceAll(strings.Split(domain, ".")[0], "_", "-")
	}
	name := "rkt-" + label + "-" + strconv.FormatInt(time.Now().UnixNano()%10000, 10)
	out["setup_url"] = origin + "/_rkt/new-app?name=" + url.QueryEscape(name) +
		"&domain=" + url.QueryEscape(domain) + "&local=" + boolStr(local) + "&tls=" + boolStr(tls)
	out["local"] = local
	cfg := p.live.Get()
	if cfg.Github == nil {
		return out, nil
	}
	out["configured"] = true
	out["app_id"] = cfg.Github.AppID
	out["app_slug"] = cfg.Github.AppSlug
	out["account"] = p.db.GetSetting("github_account")
	out["repository_selection"] = p.db.GetSetting("github_repo_selection")
	out["install_url"] = p.db.GetSetting("github_install_url")
	if out["install_url"] == "" && cfg.Github.AppSlug != "" {
		out["install_url"] = "https://github.com/apps/" + cfg.Github.AppSlug + "/installations/new"
	}
	out["repositories"] = p.db.GetSetting("github_repo_count")
	if p.gh() == nil {
		return out, nil
	}
	insts, err := p.gh().Installations()
	if err != nil {
		out["error"] = err.Error()
		return out, nil
	}
	out["installations"] = len(insts)
	if len(insts) == 0 {
		out["repository_selection"] = ""
		return out, nil
	}
	inst := insts[0]
	id := inst.ID
	if want := p.db.GetSetting("github_install_id"); want != "" {
		for _, i := range insts {
			if strconv.FormatInt(i.ID, 10) == want {
				id = i.ID
				break
			}
		}
	}
	out["account"] = inst.Account.Login
	out["repository_selection"] = inst.RepositorySelection
	out["install_url"] = inst.HTMLURL
	out["install_target"] = inst.TargetType
	if tok, err := p.gh().TokenFor(id); err == nil {
		if repos, err := p.gh().Repositories(tok); err == nil {
			out["repositories"] = strconv.Itoa(len(repos))
		}
	}
	return out, nil
}

// githubRepos lista repos de todas las instalaciones (personales y orgs).
func (p *Program) githubRepos() ([]github.Repo, error) {
	gh := p.gh()
	if gh == nil {
		return nil, fmt.Errorf("sin github app")
	}
	insts, err := gh.Installations()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []github.Repo{}
	for _, inst := range insts {
		tok, err := gh.TokenFor(inst.ID)
		if err != nil {
			continue
		}
		repos, err := gh.Repositories(tok)
		if err != nil {
			continue
		}
		for _, r := range repos {
			if seen[r.FullName] {
				continue
			}
			seen[r.FullName] = true
			out = append(out, github.Repo{
				FullName: r.FullName, DefaultBranch: r.DefaultBranch,
				Private: r.Private, Owner: r.Owner.Login,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	return out, nil
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
	return out
}

// probeApp verifica HTTP al puerto de la app (cache 3s: evita martillar
// con el polling del dashboard). Sin puerto no hay probe.
func (p *Program) probeApp(name string, live *config.Config) bool {
	var port string
	for i := range live.Apps {
		if live.Apps[i].Name == name {
			port = live.Apps[i].Env["PORT"]
		}
	}
	if port == "" {
		return true
	}
	p.probeMu.Lock()
	if v, ok := p.probes[name]; ok && time.Since(v.at) < 3*time.Second {
		p.probeMu.Unlock()
		return v.ok
	}
	p.probeMu.Unlock()

	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + port + "/")
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

// supCtl adapta Manager a deploy.Controller.
type supCtl struct {
	sup *supervisor.Manager
}

func (c *supCtl) SwapAndRestart(name string, swap func() error) error {
	return c.sup.SwapAndRestart(name, swap)
}

func (c *supCtl) Snapshot() []supervisor.Status {
	return c.sup.Snapshot()
}

// nilFS FS vacío (dist ausente): todo Stat falla -> dashboard 503.
type nilFS struct{}

func (nilFS) Open(name string) (fs.File, error) { return nil, fs.ErrNotExist }

// deployApp ejecuta un deploy en el servicio: token del entorno del
// SERVICIO (variable de máquina para SYSTEM), log a archivo.
// actor queda en historial (usuario api, hook, discover, auto, local).
func (p *Program) deployApp(name, actor string) (string, error) {
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

// buildLocal compila una app de directorio local con su plan (buildpack):
// registra la salida como deployment en curso, corre install+build y
// reinicia la unidad al terminar. Devuelve el id para seguir el log.
func (p *Program) buildLocal(name, actor string) (int64, error) {
	if err := p.beginOp(name); err != nil {
		return 0, err
	}
	release := true
	defer func() {
		if release {
			p.endOp(name)
		}
	}()
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
	plan, err := buildpack.DetectDir(app.Dir)
	if err != nil {
		return 0, err
	}
	steps := append(append([]string{}, plan.Install...), plan.Build...)
	if len(steps) == 0 {
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
		depID, _ = p.db.InsertDeployment(app.Name, "local", out.Name(), actor, target.Env)
	}
	release = false
	go func() {
		defer p.endOp(target.Name)
		defer out.Close()
		err := deploy.RunSteps(steps, target.Dir, target.Env, out)
		if err == nil && p.sup != nil {
			// el build no basta: la unidad debe responder HTTP tras reiniciar
			p.sup.Restart(target.Name)
			if perr := deploy.WaitAppHTTP(target, 10*time.Second); perr != nil {
				err = fmt.Errorf("build ok pero la app no responde: %w", perr)
			}
		}
		status := "ok"
		if err != nil {
			status = "failed"
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

// deployAppAsync arranca el deploy en segundo plano y devuelve el id de la
// salida para seguirlo en vivo (creación de apps estilo Vercel).
func (p *Program) deployAppAsync(name, actor string) (int64, error) {
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
	if err := p.beginOp(app.Name); err != nil {
		return 0, "", err
	}
	release := true
	defer func() {
		if release {
			p.endOp(app.Name)
		}
	}()
	token, err := p.appToken(&app)
	if err != nil {
		return 0, "", err
	}
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
		depID, _ = p.db.InsertDeployment(app.Name, "", out.Name(), actor, app.Env)
	}
	finish := func(rev string, runErr error) {
		if p.db != nil && depID > 0 {
			status := "ok"
			if runErr != nil {
				status = "failed"
			}
			if ferr := p.db.FinishDeployment(depID, status, prevRev, rev); ferr != nil {
				p.log.Warn("historial no actualizado", "err", ferr)
			}
		}
	}
	if async {
		release = false
		go func() {
			defer p.endOp(app.Name)
			defer out.Close()
			rev, runErr := deploy.Run(app, token, &supCtl{sup: p.sup}, out, p.log)
			finish(rev, runErr)
		}()
		return depID, "", nil
	}
	defer out.Close()
	rev, err := deploy.Run(app, token, &supCtl{sup: p.sup}, out, p.log)
	finish(rev, err)
	if err != nil {
		return depID, "", err
	}
	return depID, rev, nil
}

// appToken resuelve token para el repo: GitHub App (instalación que cubre
// el repo) si hay manager y es github.com; si no, PAT de token_env.
func (p *Program) appToken(app *config.App) (string, error) {
	if !deploy.NeedsAuth(app.Deploy.Repo) {
		return "", nil
	}
	if p.gh() != nil {
		owner, repo, ok := github.ParseRepo(app.Deploy.Repo)
		if !ok {
			return "", fmt.Errorf("repo no parseable %q", app.Deploy.Repo)
		}
		id, _, err := p.gh().InstallationForRepo(owner, repo)
		if err != nil {
			return "", fmt.Errorf("instalación para %s/%s: %w (revisa que la App esté instalada con acceso a todos los repos)", owner, repo, err)
		}
		tok, err := p.gh().TokenFor(id)
		if err != nil {
			return "", fmt.Errorf("token github app: %w", err)
		}
		return tok, nil
	}
	if token := os.Getenv(app.Deploy.TokenEnv); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("falta token en env %s (variable de máquina para el servicio)", app.Deploy.TokenEnv)
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
	var depID int64
	if p.db != nil {
		depID, _ = p.db.InsertDeployment(app.Name, "", "", actor, app.Env)
	}
	err := deploy.Rollback(*app, &supCtl{sup: p.sup}, p.log)
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
func (p *Program) alertLoop(live *config.Live, db *store.DB) {
	_ = live
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		rules, err := db.ListRules()
		if err != nil || len(rules) == 0 {
			continue
		}
		now := time.Now().UTC()
		evs, _ := db.ListEvents(500, "", "")
		for _, r := range rules {
			since := now.Add(-time.Duration(r.WindowMin) * time.Minute)
			failed := 0
			for _, e := range evs {
				t, err := time.Parse(time.RFC3339, e.Ts)
				if err != nil || t.Before(since) {
					continue
				}
				if r.App != "*" && e.App != r.App {
					continue
				}
				if e.Kind == "died" || e.Kind == "failed" {
					failed++
				}
			}
			rate := 0.0
			if r.Kind == "error_rate" {
				total, errs := 0, 0
				for _, e := range weblog.ParseFile(filepath.Join(logx.DirFor(p.cfgPath), "access.log")) {
					if e.Time.Before(since) {
						continue
					}
					total++
					if e.Status >= 500 {
						errs++
					}
				}
				if total > 0 {
					rate = float64(errs) / float64(total)
				}
			}
			if alerts.Fired(r, failed, rate, now) {
				msg := alerts.Describe(r)
				if err := db.InsertEvent("alert", r.App, msg); err == nil {
					_ = db.MarkFired(r.ID, now)
					p.log.Warn("alerta disparada", "regla", r.ID, "msg", msg)
				}
			}
		}
	}
}

// runDiscovery registra repos nuevos de todas las instalaciones.
func (p *Program) runDiscovery() {
	if p.db == nil || p.gh() == nil {
		return
	}
	if _, err := discover.Run(discover.Deps{
		GH: p.gh(), DB: p.db, Log: p.log,
		DeployFn: func(name, actor string) (string, error) { return p.deployApp(name, actor) },
	}); err != nil {
		p.log.Warn("discovery fallida", "err", err)
	}
}

// autoDeploy sondea GitHub por cambios y despliega (push-to-deploy con
// delay de poll). El git local es el estado: si HEAD != remoto, deploy.
// El intervalo se recalcula: tras crear la App la config llega por hot-reload.
func (p *Program) autoDeploy(live *config.Live) {
	next := 2 * time.Second
	var lastPoll time.Time
	for {
		wait := next
		if gh := live.Get().Github; gh != nil && gh.Poll() > 0 && !lastPoll.IsZero() {
			wait = gh.Poll()
		}
		select {
		case <-time.After(wait):
		case <-p.discoverKick:
		}
		cfg := live.Get()
		if cfg.Github == nil || p.gh() == nil {
			next = 2 * time.Second
			continue
		}
		p.runDiscovery()
		cfg = live.Get()
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
			tok, err := p.appToken(a)
			if err != nil {
				p.log.Warn("auto-deploy: sin token", "app", a.Name, "err", err)
				continue
			}
			remote, err := p.gh().CommitSHA(owner, repo, a.Deploy.Ref, tok)
			if err != nil {
				p.log.Warn("auto-deploy: sin remoto", "app", a.Name, "err", err)
				continue
			}
			if local := deploy.HeadRev(a.Dir); local == "" || local == remote {
				continue
			}
			p.log.Info("auto-deploy: cambio detectado", "app", a.Name, "rev", shortSHA(remote))
			if rev, err := p.deployApp(a.Name, "auto"); err != nil {
				p.log.Error("auto-deploy fallido", "app", a.Name, "err", err)
			} else {
				p.log.Info("auto-deploy ok", "app", a.Name, "rev", rev)
			}
		}
		lastPoll = time.Now()
		if cfg.Github.Poll() > 0 {
			next = cfg.Github.Poll()
		} else {
			next = time.Minute
		}
	}
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// watchMerged recarga la flota cuando cambia la DB (única fuente).
// Fuente inválida = se conserva la anterior.
func (p *Program) watchMerged(live *config.Live, db *store.DB) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	lastVer := db.ConfigVersion()
	for range t.C {
		if v := db.ConfigVersion(); v == lastVer {
			continue
		} else {
			lastVer = v
		}
		base, err := cfgfile.LoadBase(db, p.cfgPath)
		if err != nil {
			p.log.Warn("infra inválida en DB, conservo anterior", "err", err)
			continue
		}
		next, err := cfgfile.Snapshot(db, base)
		if err != nil {
			p.log.Warn("snapshot inválido, conservo anterior", "err", err)
			continue
		}
		if err := cfgfile.OverlayGithub(next, db); err != nil {
			p.log.Warn("github en DB inválido, conservo anterior", "err", err)
			continue
		}
		old := live.Get()
		live.Swap(next)
		if p.sup != nil {
			// manifiesto en caliente: nuevas/actualizadas arrancan solas
			p.sup.Apply(next.Apps, next.GlobalEnv)
		}
		if next.Github != nil && p.gh() == nil {
			if gh, err := github.New(next.Github.AppID, next.Github.KeyFile, ""); err == nil {
				p.ghm.Store(gh)
			}
		}
		if next.Github != nil && p.autoStarted.CompareAndSwap(false, true) {
			go p.autoDeploy(live)
		}
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
