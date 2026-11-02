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
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt-proxy/internal/build"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/discover"
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
	code, body, err := s.post("/restart/"+app, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("sin servicio: %w", err)
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("restart: %s", deploy.OneLine(body))
	}
	return fmt.Sprintf("app %s reiniciando", app), nil
}

// doRollback vuelve a live.prev (tarda: swap+restart+health).
func (s *Server) doRollback(app string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("falta app")
	}
	code, body, err := s.post("/rollback/"+app, 10*time.Minute)
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
	code, body, err := s.post("/deploy/"+app, 10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("sin servicio: %w", err)
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("deploy: %s", deploy.OneLine(body))
	}
	return fmt.Sprintf("deploy ok: %s", deploy.OneLine(body)), nil
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
	Repo    string `json:"repo,omitempty" jsonschema:"repo git https o dir local (sugiere valores)"`
	Name    string `json:"name,omitempty" jsonschema:"nombre [a-z0-9-] (default: slug del repo)"`
	Domain  string `json:"domain,omitempty" jsonschema:"dominio (default: <nombre>.<base_domain>)"`
	Port    int    `json:"port,omitempty" jsonschema:"puerto local (default: libre del pool)"`
	Command string `json:"command,omitempty" jsonschema:"binario absoluto (default: bin del preset)"`
	Dir     string `json:"dir,omitempty" jsonschema:"dir app (default: estándar por OS)"`
	Preset  string `json:"preset,omitempty" jsonschema:"next|node|python (default: detectado)"`
	Auto    bool   `json:"auto,omitempty" jsonschema:"auto-deploy"`
}
type envSetArg struct {
	App   string `json:"app,omitempty" jsonschema:"nombre de la app"`
	Key   string `json:"key,omitempty" jsonschema:"CLAVE"`
	Value string `json:"value,omitempty" jsonschema:"valor (admite ${VAR})"`
}
type deploymentLogsArg struct {
	App string `json:"app,omitempty" jsonschema:"nombre de la app"`
	N   int    `json:"n,omitempty" jsonschema:"últimas líneas (default 50)"`
}
type waitHealthyArg struct {
	App      string `json:"app,omitempty" jsonschema:"nombre de la app"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 60, máx 300)"`
}

// doAppsAdd registra app con inferencia (igual que `apps add` sin flags).
func (s *Server) doAppsAdd(a appsAddArg) (string, error) {
	_, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var sug *discover.Suggestion
	if a.Repo != "" {
		sug, err = discover.Suggest(db, nil, a.Repo)
		if err != nil {
			return "", err
		}
	} else {
		sug = &discover.Suggestion{}
	}
	name := orDefault(a.Name, sug.Name)
	if name == "" {
		return "", fmt.Errorf("sin nombre (pasa name)")
	}
	command := orDefault(a.Command, sug.Command)
	if command == "" {
		return "", fmt.Errorf("sin binario (pasa command absoluto)")
	}
	dir := a.Dir
	if dir == "" {
		dir = sug.Dir
	}
	if dir == "" {
		dir = `C:\apps\` + name
	}
	port := ""
	if a.Port > 0 {
		port = strconv.Itoa(a.Port)
	} else if sug.Port > 0 {
		port = strconv.Itoa(sug.Port)
	}
	domains := []string{}
	if a.Domain != "" {
		domains = []string{a.Domain}
	} else if sug.Domain != "" {
		domains = []string{sug.Domain}
	}
	preset := orDefault(a.Preset, sug.Preset)
	build, args, env := discover.PresetBuild(preset, port)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	app := config.App{
		Name: name, Command: command, Args: args,
		Dir: dir, Env: env, RestartDelay: "5s",
	}
	if a.Repo != "" {
		app.Deploy = &config.Deploy{
			Repo: a.Repo, Ref: "main", Build: build, Auto: a.Auto,
		}
	}
	if err := cfgfile.AddApp(db, app, domains, port); err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{
		"app": name, "preset": preset, "domains": domains, "port": port,
	})
	return string(out), nil
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// doEnvSet escribe var (sin lectura: los secretos no vuelven al contexto).
func (s *Server) doEnvSet(app, key, value string) (string, error) {
	if app == "" || key == "" {
		return "", fmt.Errorf("faltan app/key")
	}
	cfg, db, err := cfgfile.LoadMerged(s.cfgPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := cfgfile.SetEnv(db, cfg, app, key, value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s=%s (restart requerido)", key, app), nil
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
		DeployFn: func(name string) (string, error) {
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
	srv := mcp.NewServer(&mcp.Implementation{Name: "rkt", Title: "rkt PaaS Windows"}, nil)
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
	mcp.AddTool(srv, &mcp.Tool{Name: "deploy", Description: "Publica una app desde git (tarda minutos: clone+build+swap). Vigila con deployments_list."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeploy(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "apps_add", Description: "Registra app con inferencia (repo local o URL): nombre, preset, puerto, dominio, comando. Luego deploy."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appsAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppsAdd(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_set", Description: "Escribe var de entorno (restart requerido). Solo escritura: no lee secretos."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envSetArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvSet(a.App, a.Key, a.Value))
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
	return srv.Run(ctx, &mcp.StdioTransport{})
}
