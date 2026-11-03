// Mutaciones de flota compartidas por CLI y MCP. Toda escritura valida
// antes de persistir (ImportFleet atómico).
package cfgfile

import (
	"fmt"
	"os"
	"strconv"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/store"
)

// AddApp valida y persiste app+routes.
func AddApp(db *store.DB, app config.App, domains []string, port string) error {
	fleet, err := db.LoadFleet()
	if err != nil {
		return err
	}
	for _, a := range fleet.Apps {
		if a.Name == app.Name {
			return fmt.Errorf("apps: %q ya existe", app.Name)
		}
	}
	if app.RestartDelay == "" {
		app.RestartDelay = "5s"
	}
	if app.MinUptime == "" {
		app.MinUptime = "10s"
	}
	if err := app.ValidateApp(len(fleet.Apps)); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	routes := fleet.Routes
	for _, h := range domains {
		if port == "" {
			return fmt.Errorf("domain sin port")
		}
		routes = append(routes, config.Route{
			Hosts: []string{h}, Target: "http://127.0.0.1:" + port,
		})
	}
	if err := config.ValidateRoutes(routes); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	fleet.Apps = append(fleet.Apps, app)
	fleet.Routes = routes
	return db.ImportFleet(fleetToConfig(fleet))
}

// fleetToConfig arma Config válida desde flota (ya validada por partes).
func fleetToConfig(fleet *store.FleetData) *config.Config {
	cfg := &config.Config{Apps: fleet.Apps, Routes: fleet.Routes}
	cfg.ApplyDefaults()
	return cfg
}

// SetEnv escribe var expandida en la app.
func SetEnv(db *store.DB, cfg *config.Config, appName, key, rawVal string) error {
	a := findAppOrNil(cfg, appName)
	if a == nil {
		return fmt.Errorf("app %q inexistente", appName)
	}
	if a.Env == nil {
		a.Env = map[string]string{}
	}
	// expande solo el valor nuevo (el resto ya está expandido en DB)
	scope := dotenv.EnvironMap()
	for gk, gv := range cfg.GlobalEnv { // las globales también resuelven ${...}
		if _, exists := scope[gk]; !exists {
			scope[gk] = gv
		}
	}
	for ek, ev := range a.Env {
		if ek != key {
			scope[ek] = ev
		}
	}
	if a.EnvFile != "" {
		raw, err := dotenv.ParseFile(a.EnvFile)
		if err != nil {
			return err
		}
		for fk, fv := range dotenv.Expand(raw, dotenv.EnvironMap()) {
			if _, exists := scope[fk]; !exists {
				scope[fk] = fv
			}
		}
	}
	a.Env[key] = dotenv.Expand(map[string]string{key: rawVal}, scope)[key]
	if err := a.ValidateApp(0); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	return db.ImportFleet(cfg)
}

// SetGlobalEnv escribe una variable global de flota en la infra (expandida
// contra el entorno del servicio y las demás globales).
func SetGlobalEnv(db *store.DB, cfg *config.Config, key, rawVal string) error {
	if key == "" {
		return fmt.Errorf("clave vacía")
	}
	if cfg.GlobalEnv == nil {
		cfg.GlobalEnv = map[string]string{}
	}
	scope := dotenv.EnvironMap()
	for k, v := range cfg.GlobalEnv {
		if k != key {
			scope[k] = v
		}
	}
	cfg.GlobalEnv[key] = dotenv.Expand(map[string]string{key: rawVal}, scope)[key]
	return SaveInfra(db, cfg)
}

// UnsetGlobalEnv borra una global de la infra.
func UnsetGlobalEnv(db *store.DB, cfg *config.Config, key string) error {
	delete(cfg.GlobalEnv, key)
	return SaveInfra(db, cfg)
}

// execOwn inyectable (test de fallo al resolver el binario).
var execOwn = os.Executable

// CreateApp arma App+domains+port con inferencia (Suggest+PresetBuild).
// Unico hogar (CLI wizard pregunta aparte; MCP y API la usan).
// Crea el dir. No escribe en DB (eso es AddApp).
func CreateApp(db *store.DB, repo, name, domain string, port int, command, dir string, auto bool) (config.App, []string, string, error) {
	var sug *discover.Suggestion
	if repo != "" {
		var err error
		sug, err = discover.Suggest(db, nil, repo)
		if err != nil {
			return config.App{}, nil, "", err
		}
	} else if dir != "" {
		// directorio local: misma inferencia que un repo (preset, nombre, puerto)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return config.App{}, nil, "", fmt.Errorf("directorio no existe: %s", dir)
		}
		var err error
		sug, err = discover.Suggest(db, nil, dir)
		if err != nil {
			return config.App{}, nil, "", err
		}
	} else {
		sug = &discover.Suggestion{}
	}
	if name == "" {
		name = sug.Name
	}
	if name == "" {
		return config.App{}, nil, "", fmt.Errorf("sin nombre (pasa name)")
	}
	if dir == "" {
		dir = sug.Dir
	}
	if dir == "" {
		dir = `C:\apps\` + name
	}
	portS := ""
	if port > 0 {
		portS = strconv.Itoa(port)
	} else if sug.Port > 0 {
		portS = strconv.Itoa(sug.Port)
	}
	domains := []string{}
	if domain != "" {
		domains = []string{domain}
	} else if sug.Domain != "" {
		domains = []string{sug.Domain}
	}

	// Plan canónico (buildpack): estático o proceso + pasos de build.
	plan := sug.Plan
	args := []string{}
	staticDir := ""
	if command == "" && plan.IsStatic() {
		exe, err := execOwn()
		if err != nil {
			return config.App{}, nil, "", fmt.Errorf("binario propio: %w", err)
		}
		command = exe
		staticDir = plan.Static(dir)
		args = []string{"__static", staticDir}
	} else if command == "" && len(plan.Start) > 0 {
		command = processBin(plan.Start[0], db)
		args = append(args, plan.Start[1:]...)
	}
	if command == "" {
		if plan.Kind != "" {
			return config.App{}, nil, "", fmt.Errorf("plan %q sin arranque: define comando", plan.Kind)
		}
		return config.App{}, nil, "", fmt.Errorf("sin plan de build (define comando o añade package.json/go.mod/requirements.txt/index.html)")
	}
	env := map[string]string{}
	if portS != "" {
		env["PORT"] = portS
	}
	build := append(append([]string{}, plan.Install...), plan.Build...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return config.App{}, nil, "", err
	}
	app := config.App{
		Name: name, Command: command, Args: args, Static: staticDir,
		Dir: dir, Env: env, RestartDelay: "5s",
	}
	if repo != "" {
		app.Deploy = &config.Deploy{
			Repo: repo, Ref: "main", Build: build, Auto: auto,
		}
	}
	return app, domains, portS, nil
}

// processBin respeta node_bin/python_bin de Ajustes; resto va por PATH.
func processBin(bin string, db *store.DB) string {
	switch bin {
	case "node":
		if v := db.GetSetting(store.KeyNodeBin); v != "" {
			return v
		}
	case "python", "python3":
		if v := db.GetSetting(store.KeyPythonBin); v != "" {
			return v
		}
	}
	return bin
}

func findAppOrNil(cfg *config.Config, name string) *config.App {
	return FindApp(cfg, name)
}

// FindApp busca app por nombre, nil si falta. Canónica; CLI la usa.
func FindApp(cfg *config.Config, name string) *config.App {
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == name {
			return &cfg.Apps[i]
		}
	}
	return nil
}
