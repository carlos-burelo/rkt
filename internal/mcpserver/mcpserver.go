// Package mcpserver expone rkt como servidor MCP (stdio) para agentes.
// Límite deliberado: opera (status, logs, restart, deploy...), no muta
// config (apps add/rm, env... eso lo hace el operador por CLI).
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/build"
	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

// githubNew abre manager App desde config efectiva (nil si no hay).
func githubNew(cfg *config.Config) (*github.Manager, error) {
	if cfg.Github == nil {
		return nil, fmt.Errorf("sin github app configurada")
	}
	return github.New(cfg.Github.AppID, cfg.Github.KeyFile, "")
}

// healthBaseOf dirección healthz (requiere activado).
func healthBaseOf(cfg *config.Config) (string, error) {
	if !cfg.Healthz.Enabled {
		return "", fmt.Errorf("healthz desactivado")
	}
	return "http://" + cfg.Healthz.Listen, nil
}

// postDeploy dispara deploy en el servicio y espera (tarda minutos).
func postDeploy(base, name string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Post(base+"/deploy/"+name, "application/json", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deploy: %s", deploy.OneLine(body))
	}
	return deploy.OneLine(body), nil
}

// Server configurable por ruta de config.
type Server struct {
	cfgPath string
	client  *http.Client
}

// New crea el servidor MCP.
func New(cfgPath string) *Server {
	return &Server{cfgPath: cfgPath, client: &http.Client{Timeout: 30 * time.Second}}
}

// healthBase lee la dirección healthz de la config.
func (s *Server) healthBase() (string, error) {
	cfg, err := cfgfile.Load(s.cfgPath)
	if err != nil {
		return "", err
	}
	return healthBaseOf(cfg)
}

func (s *Server) get(path string) (int, []byte, error) {
	base, err := s.healthBase()
	if err != nil {
		return 0, nil, err
	}
	resp, err := s.client.Get(base + path)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, err
}

func (s *Server) post(path string, timeout time.Duration) (int, []byte, error) {
	base, err := s.healthBase()
	if err != nil {
		return 0, nil, err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(base+path, "application/json", nil)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, err
}

// doStatus devuelve el /healthz tal cual.
func (s *Server) doStatus() (string, error) {
	code, body, err := s.get("/healthz")
	if err != nil {
		return "", fmt.Errorf("sin servicio: %w", err)
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("healthz %d", code)
	}
	return string(body), nil
}

// doLogs tail del log operativo o de app.
func (s *Server) doLogs(n int, app string) (string, error) {
	path := logx.Path(s.cfgPath)
	if app != "" {
		path = filepath.Join(logx.DirFor(s.cfgPath), "apps", app+".log")
	}
	return logx.Tail(path, n)
}

// doDeployments historial SQLite.
func (s *Server) doDeployments(n int, app string) (string, error) {
	if n < 1 {
		n = 20
	}
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		return "", err
	}
	defer db.Close()
	deps, err := db.ListDeployments(app, n)
	if err != nil {
		return "", err
	}
	if deps == nil {
		deps = []store.Deployment{}
	}
	out, err := json.Marshal(deps)
	return string(out), err
}

// doEvents auditoría de crashes del supervisor.
func (s *Server) doEvents(n int, kind, app string) (string, error) {
	if n < 1 {
		n = 20
	}
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		return "", err
	}
	defer db.Close()
	evs, err := db.ListEvents(n, kind, app)
	if err != nil {
		return "", err
	}
	if evs == nil {
		evs = []store.Event{}
	}
	out, err := json.Marshal(evs)
	return string(out), err
}

// doRestart reinicia app (inmediato, sin cortes por restart_grace).
func (s *Server) doRestart(app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	// Una app recién dada de alta puede no estar en el supervisor hasta
	// que el servicio recarga el manifiesto (~10s): reintenta sin fallar.
	deadline := time.Now().Add(15 * time.Second)
	var last string
	for {
		code, body, err := s.post("/restart/"+app, 30*time.Second)
		if err == nil && code == http.StatusOK {
			return fmt.Sprintf("app %s reiniciando", app), nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = deploy.OneLine(body)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("restart: %s", last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// postApplied reintenta 404 mientras el servicio aplica un alta reciente
// (watcher ≤10s). Errores de red no se reintentan.
func (s *Server) postApplied(path string, timeout time.Duration) (int, []byte, error) {
	deadline := time.Now().Add(15 * time.Second)
	for {
		code, body, err := s.post(path, timeout)
		if err != nil {
			return 0, nil, err
		}
		if code != http.StatusNotFound || time.Now().After(deadline) {
			return code, body, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// doRollback vuelve a live.prev (tarda: swap+restart+health).
func (s *Server) doRollback(app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	code, body, err := s.postApplied("/rollback/"+app, 10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("sin servicio: %w", err)
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("rollback: %s", deploy.OneLine(body))
	}
	return fmt.Sprintf("rollback ok: %s", app), nil
}

// doDeploy publica app (tarda minutos: clone+build+swap; vigila con deployments_list).
func (s *Server) doDeploy(app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	code, body, err := s.postApplied("/deploy/"+app, 10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("sin servicio: %w", err)
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("deploy: %s", deploy.OneLine(body))
	}
	return fmt.Sprintf("deploy ok: %s", deploy.OneLine(body)), nil
}

// doRetry re-corre el rev exacto de un deploy (tarda minutos).
func (s *Server) doRetry(id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("falta id")
	}
	code, body, err := s.postApplied(fmt.Sprintf("/retry/%d", id), 10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("sin servicio: %w", err)
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("retry: %s", deploy.OneLine(body))
	}
	return fmt.Sprintf("retry ok: %s", deploy.OneLine(body)), nil
}

func text(s string, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "error: " + err.Error()}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}, nil, nil
}

type noArgs struct{}
type appArg struct {
	App string `json:"app,omitempty" jsonschema:"nombre de la app"`
}
type retryArg struct {
	ID int `json:"id,omitempty" jsonschema:"id del deployment a repetir"`
}
type logsArg struct {
	N   int    `json:"n,omitempty" jsonschema:"últimas líneas (default 50)"`
	App string `json:"app,omitempty" jsonschema:"app para su log (vacío = operativo)"`
}
type deploymentsArg struct {
	N   int    `json:"n,omitempty" jsonschema:"filas (default 20)"`
	App string `json:"app,omitempty" jsonschema:"filtra por app (vacío = todas)"`
}
type eventsArg struct {
	N    int    `json:"n,omitempty" jsonschema:"filas (default 20)"`
	Kind string `json:"kind,omitempty" jsonschema:"filtra por tipo: died|failed (vacío = todos)"`
	App  string `json:"app,omitempty" jsonschema:"filtra por app (vacío = todas)"`
}
type appsAddArg struct {
	Repo    string   `json:"repo,omitempty" jsonschema:"repo git https o dir local (sugiere valores)"`
	Name    string   `json:"name,omitempty" jsonschema:"nombre [a-z0-9-] (default: slug del repo)"`
	Runtime string   `json:"runtime,omitempty" jsonschema:"managed (default) | external (solo proxy, sin proceso)"`
	Domains []string `json:"domains,omitempty" jsonschema:"dominios iniciales (default local: <nombre>.localhost; público: <nombre>.<base_domain>)"`
	Port    int      `json:"port,omitempty" jsonschema:"puerto local (default: libre del pool)"`
	Command string   `json:"command,omitempty" jsonschema:"binario absoluto (default: detectado del plan)"`
	Dir     string   `json:"dir,omitempty" jsonschema:"dir app (default: estándar por OS)"`
	Auto    bool     `json:"auto,omitempty" jsonschema:"auto-deploy"`
}
type envSetArg struct {
	App   string `json:"app,omitempty" jsonschema:"nombre de la app"`
	Key   string `json:"key,omitempty" jsonschema:"CLAVE"`
	Value string `json:"value,omitempty" jsonschema:"valor (admite ${VAR})"`
}
type envImportArg struct {
	App string `json:"app,omitempty" jsonschema:"app (vacío = global)"`
	Raw string `json:"raw" jsonschema:"contenido .env"`
}
type deploymentLogsArg struct {
	App string `json:"app,omitempty" jsonschema:"nombre de la app"`
	N   int    `json:"n,omitempty" jsonschema:"últimas líneas (default 50)"`
}
type waitHealthyArg struct {
	App      string `json:"app,omitempty" jsonschema:"nombre de la app"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 60, máx 300)"`
}
type dirArg struct {
	Dir string `json:"dir" jsonschema:"directorio del proyecto a analizar"`
}
type removeAppArg struct {
	App        string `json:"app" jsonschema:"nombre de la app"`
	KeepRoutes bool   `json:"keep_routes,omitempty" jsonschema:"conservar rutas huérfanas (default: borrarlas)"`
}
type appUpdateArg struct {
	App        string    `json:"app" jsonschema:"nombre de la app"`
	Domains    *[]string `json:"domains,omitempty" jsonschema:"hosts de la app (vacío = quitar todos)"`
	Port       *int      `json:"port,omitempty" jsonschema:"puerto local (retargeta rutas)"`
	Static     *string   `json:"static,omitempty" jsonschema:"dir servido como estático"`
	EnvFile    *string   `json:"env_file,omitempty" jsonschema:"dotenv adicional"`
	Command    *string   `json:"command,omitempty" jsonschema:"binario de arranque"`
	Dir        *string   `json:"dir,omitempty" jsonschema:"directorio de trabajo"`
	Args       *[]string `json:"args,omitempty" jsonschema:"argumentos del proceso"`
	MemoryMB   *uint64   `json:"memory_mb,omitempty" jsonschema:"límite RAM en MB (0 = sin tope)"`
	MinUptime  *string   `json:"min_uptime,omitempty" jsonschema:"uptime mínimo sano (ej 10s)"`
	Disabled   *bool     `json:"disabled,omitempty" jsonschema:"app detenida (true) o activa (false)"`
	HealthPath *string   `json:"health_path,omitempty" jsonschema:"ruta de salud (default /)"`
}

// doAppsAdd registra app con inferencia (igual que `apps add` sin flags).
func (s *Server) doAppsAdd(a appsAddArg) (string, error) {
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	gh, _ := githubNew(cfg)
	app, domains, port, err := cfgfile.CreateAndAdd(db, gh, a.Runtime, a.Repo, a.Name, a.Domains, a.Port, a.Command, a.Dir, a.Auto)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{
		"app": app.Name, "domains": domains, "port": port,
	})
	return string(out), nil
}

// doEnvImport aplica un .env con el parser canónico.
func (s *Server) doEnvImport(a envImportArg) (string, error) {
	parsed, err := dotenv.Parse(a.Raw)
	if err != nil {
		return "", err
	}
	_, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if a.App == "" {
		_, err = cfgfile.MutateInfra(db, func(cp *config.Config) error {
			return cfgfile.ApplyEnvPatch(cp, "", parsed, nil)
		})
	} else {
		_, err = cfgfile.MutateFleet(db, func(cp *config.Config) error {
			return cfgfile.ApplyEnvPatch(cp, a.App, parsed, nil)
		})
	}
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b, _ := json.Marshal(map[string]any{"ok": true, "keys": keys})
	return string(b), nil
}

// doEnvSet escribe var (sin lectura: los secretos no vuelven al contexto).
func (s *Server) doEnvSet(app, key, value string) (string, error) {
	if app == "" || key == "" {
		return "", fmt.Errorf("faltan app/key")
	}
	_, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := cfgfile.SetEnv(db, app, key, value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s guardada (reinicia para aplicar)", app, key), nil
}

// doDiscover corre auto-registro ahora (sin esperar el poll).
// Devuelve apps registradas en esta pasada.
func (s *Server) doDiscover() (string, error) {
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if cfg.Github == nil {
		return "", fmt.Errorf("sin github app configurada")
	}
	gh, err := githubNew(cfg)
	if err != nil {
		return "", err
	}
	base, err := healthBaseOf(cfg)
	if err != nil {
		return "", err
	}
	registered, err := discover.Run(discover.Deps{
		GH: gh, DB: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DeployFn: func(name, actor string) (string, error) {
			return postDeploy(base, name)
		},
	})
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{"registered": registered})
	return string(out), nil
}
func (s *Server) doDeployCheck(app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	checks := map[string]any{"app": false, "deploy": false, "git": false, "dir": false, "servicio": false}
	for i := range cfg.Apps {
		if cfg.Apps[i].Name != app {
			continue
		}
		a := &cfg.Apps[i]
		checks["app"] = true
		if a.Deploy != nil {
			checks["deploy"] = true
		}
		if _, err := exec.LookPath("git"); err == nil {
			checks["git"] = true
		}
		if st, err := os.Stat(a.Dir); err == nil && st.IsDir() {
			checks["dir"] = true
		}
		break
	}
	if _, _, err := s.get("/healthz"); err == nil {
		checks["servicio"] = true
	}
	out, _ := json.Marshal(checks)
	return string(out), nil
}

// doAppsList flota completa con dominio, puerto, plan y estado DNS.
func (s *Server) doAppsList() (string, error) {
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	out, err := json.Marshal(appview.Apps(cfg))
	return string(out), err
}

// doRoutesList rutas con aviso de DNS local por host.
func (s *Server) doRoutesList() (string, error) {
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	type row struct {
		Hosts  []string `json:"hosts"`
		Target string   `json:"target"`
		DNSOK  []bool   `json:"dns_ok"`
	}
	out := []row{}
	for _, r := range cfg.Routes {
		dns := make([]bool, 0, len(r.Hosts))
		for _, h := range r.Hosts {
			dns = append(dns, appview.ResolvesLocal(h))
		}
		out = append(out, row{Hosts: r.Hosts, Target: r.Target, DNSOK: dns})
	}
	b, err := json.Marshal(out)
	return string(b), err
}

// doZonesList zonas locales agrupadas (apex + subdominios).
func (s *Server) doZonesList() (string, error) {
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	b, err := json.Marshal(appview.BuildZones(cfg.Routes, db.GetSetting(store.KeyBaseDomain)))
	return string(b), err
}

// doDetectPlan plan de build inferido sin crear la app.
func (s *Server) doDetectPlan(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("falta dir")
	}
	plan, err := buildpack.DetectDir(dir)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(map[string]any{
		"dir": dir, "framework": plan.Kind, "pm": plan.PM,
		"install": plan.Install, "build": plan.Build, "start": plan.Start,
		"static_dir": plan.StaticDir, "static_abs": plan.Static(dir),
	})
	return string(b), err
}

// doBuild compila un directorio local vía el servicio; devuelve el id.
func (s *Server) doBuild(app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	// app recién registrada: el servicio la aplica en ≤10s; reintenta.
	deadline := time.Now().Add(20 * time.Second)
	var last string
	for {
		code, body, err := s.post("/build/"+app, 60*time.Second)
		if err == nil && code == http.StatusOK {
			return string(body), nil
		}
		if err != nil {
			last = err.Error()
		} else {
			last = deploy.OneLine(body)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("build: %s", last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// doAppSetDisabled detiene o arranca una app (persistente).
func (s *Server) doAppSetDisabled(name string, disabled bool) (string, error) {
	if name == "" {
		return "", fmt.Errorf("falta app")
	}
	_, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		return cfgfile.UpdateApp(cp, name, cfgfile.AppPatch{Disabled: &disabled})
	}); err != nil {
		return "", err
	}
	if disabled {
		return "app detenida (el servicio la aplica en ≤10s)", nil
	}
	return "app activa (el servicio la arranca en ≤10s)", nil
}

// doAppUpdate cambia dominio/puerto/proceso; el servicio aplica ≤10s.
func (s *Server) doAppUpdate(a appUpdateArg) (string, error) {
	if a.App == "" {
		return "", fmt.Errorf("falta app")
	}
	_, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	patch := cfgfile.AppPatch{
		Command: a.Command, Args: a.Args, Dir: a.Dir, Static: a.Static,
		EnvFile: a.EnvFile, MemoryMB: a.MemoryMB, MinUptime: a.MinUptime,
		Port: a.Port, Domains: a.Domains, Disabled: a.Disabled, HealthPath: a.HealthPath,
	}
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		return cfgfile.UpdateApp(cp, a.App, patch)
	}); err != nil {
		return "", err
	}
	return "configuración actualizada (el servicio la aplica en ≤10s)", nil
}

// doAppsRemove borra una app de la flota (y sus rutas salvo keep_routes).
// El servicio la detiene al aplicar el manifiesto (≤10s).
func (s *Server) doAppsRemove(a removeAppArg) (string, error) {
	if a.App == "" {
		return "", fmt.Errorf("falta app")
	}
	_, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	routesDropped := 0
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		n, err := cfgfile.RemoveApp(cp, a.App, a.KeepRoutes)
		routesDropped = n
		return err
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("app %s borrada (%d rutas)", a.App, routesDropped), nil
}

// doDeploymentLogs tail del log del último deploy de la app.
func (s *Server) doDeploymentLogs(app string, n int) (string, error) {
	matches, err := filepath.Glob(filepath.Join(logx.DirFor(s.cfgPath), "deploys", app+"-*.log"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("sin logs de deploy para %q", app)
	}
	latest := matches[0]
	for _, m := range matches[1:] {
		if m > latest {
			latest = m
		}
	}
	return logx.Tail(latest, n)
}

// doWaitHealthy espera app sana hasta timeout_s.
func (s *Server) doWaitHealthy(app string, timeoutS int) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	if timeoutS < 1 {
		timeoutS = 60
	}
	if timeoutS > 300 {
		timeoutS = 300
	}
	deadline := time.Now().Add(time.Duration(timeoutS) * time.Second)
	for {
		code, body, err := s.get("/healthz")
		if err == nil && code == http.StatusOK {
			var st struct {
				Apps []health.AppStatus `json:"apps"`
			}
			if json.Unmarshal(body, &st) == nil {
				for _, a := range st.Apps {
					if a.Name == app {
						if a.Running && !a.Failed {
							return fmt.Sprintf("%s sana", app), nil
						}
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s no sana tras %ds", app, timeoutS)
		}
		time.Sleep(2 * time.Second)
	}
}

// Run registra tools y sirve por stdio (bloquea).
func (s *Server) Run(ctx context.Context) error {
	srv := mcp.NewServer(&mcp.Implementation{Name: "rkt", Title: "rkt PaaS"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "status", Description: "Salud: versión, backends y apps (JSON). Requiere servicio en marcha."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doStatus())
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "logs", Description: "Tail del log operativo o de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a logsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doLogs(a.N, a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "deployments_list", Description: "Historial de deploys en SQLite."},
		func(ctx context.Context, req *mcp.CallToolRequest, a deploymentsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeployments(a.N, a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "events_list", Description: "Auditoría de crashes del supervisor (died/failed)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a eventsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEvents(a.N, a.Kind, a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "version", Description: "Versión del binario rkt."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(build.Version, nil)
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "restart", Description: "Reinicia una app al instante (sin cortes por restart_grace)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRestart(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "rollback", Description: "Vuelve una app a live.prev (tarda: swap+restart+health)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRollback(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "stop", Description: "Detiene una app de forma persistente (no arranca hasta start)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppSetDisabled(a.App, true))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "start", Description: "Arranca una app detenida y limpia disabled."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppSetDisabled(a.App, false))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "deploy", Description: "Publica una app desde git (tarda minutos: clone+build+swap). Vigila con deployments_list."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeploy(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "retry", Description: "Repite un deployment por ID con su mismo rev (tarda minutos)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a retryArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRetry(a.ID))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "apps_add", Description: "Registra app con inferencia (repo local o URL): detecta build/arranque, nombre, puerto y dominio. Luego deploy."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appsAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppsAdd(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_set", Description: "Escribe var de entorno (restart requerido). Solo escritura: no lee secretos."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envSetArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvSet(a.App, a.Key, a.Value))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_import", Description: "Importa un .env (global o de una app) con el parser canónico."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envImportArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvImport(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "discover", Description: "Auto-registro inmediato desde GitHub App (sin esperar el poll)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doDiscover())
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "deploy_check", Description: "Preflight sin efectos: app, deploy, git, dir, servicio (JSON)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeployCheck(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "deployment_logs", Description: "Tail del log del último deploy de la app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a deploymentLogsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeploymentLogs(a.App, a.N))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "wait_healthy", Description: "Espera app sana hasta timeout_s (default 60, máx 300). Para gates post-deploy."},
		func(ctx context.Context, req *mcp.CallToolRequest, a waitHealthyArg) (*mcp.CallToolResult, any, error) {
			return text(s.doWaitHealthy(a.App, a.TimeoutS))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "apps_list", Description: "Apps con dominio, puerto, estático, deploy y dns_ok (JSON)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doAppsList())
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "routes_list", Description: "Rutas host→target con dns_ok por host (JSON)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesList())
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "zones_list", Description: "Zonas locales (localhost/test/base) con apex y subdominios (JSON)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doZonesList())
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "detect_plan", Description: "Plan de build inferido para un directorio (framework, install/build/start o estático)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a dirArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDetectPlan(a.Dir))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "build", Description: "Compila una app de directorio local con su plan (tarda; sigue con deployments_list)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doBuild(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "app_update", Description: "Cambia dominio, puerto, comando, args, dir, estático, env_file o memoria de una app (aplica en ≤10s)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appUpdateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppUpdate(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "apps_remove", Description: "Borra una app registrada (detiene el proceso al aplicar, ≤10s). Por defecto borra sus rutas."},
		func(ctx context.Context, req *mcp.CallToolRequest, a removeAppArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppsRemove(a))
		})
	s.registerMore(srv)
	return srv.Run(ctx, &mcp.StdioTransport{})
}
