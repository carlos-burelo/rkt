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

// CreateApp arma App+domains+port con inferencia (Suggest+PresetBuild).
// Unico hogar (CLI wizard pregunta aparte; MCP y API la usan).
// Crea el dir. No escribe en DB (eso es AddApp).
func CreateApp(db *store.DB, repo, name, domain string, port int, command, dir, preset string, auto bool) (config.App, []string, string, error) {
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
	if preset == "" {
		preset = sug.Preset
	}
	if command == "" {
		command = sug.Command
	}
	if command == "" {
		// error accionable: dice qué falta y dónde se arregla
		if binKey, ok := discover.PresetBinKey(preset); ok {
			return config.App{}, nil, "", fmt.Errorf("sin binario para preset %q (define comando o configura %s en Ajustes)", preset, binKey)
		}
		return config.App{}, nil, "", fmt.Errorf("sin binario inferido (define comando o configura node_bin/python_bin en Ajustes)")
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
	build, args, env := discover.PresetBuild(preset, portS)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return config.App{}, nil, "", err
	}
	app := config.App{
		Name: name, Command: command, Args: args,
		Dir: dir, Env: env, RestartDelay: "5s",
	}
	if repo != "" {
		app.Deploy = &config.Deploy{
			Repo: repo, Ref: "main", Build: build, Auto: auto,
		}
	}
	return app, domains, portS, nil
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
