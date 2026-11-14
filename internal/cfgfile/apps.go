// Mutaciones de flota compartidas por CLI y MCP. Toda escritura valida
// antes de persistir (ImportFleet atómico).
package cfgfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/domains"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/platform"
	"rkt-proxy/internal/store"
)

// CreateAndAdd es el alta canónica: infiere (repo/dir) o registra externa,
// valida y persiste app+rutas. API, MCP y CLI no reimplementan el flujo.
// domains vacío deja que la inferencia elija el dominio inicial.
// appsBase: padre de apps (home/apps); vacío = default por plataforma.
func CreateAndAdd(db *store.DB, gh *github.Manager, runtime, repo, name string, domains []string, port int, command, dir string, auto bool, appsBase string) (config.App, []string, string, error) {
	var (
		app     config.App
		initial []string
		portS   string
		err     error
	)
	primary := ""
	if len(domains) > 0 {
		primary = domains[0]
	}
	if runtime == config.RuntimeExternal {
		app, initial, portS, err = CreateExternalApp(db, name, primary, dir, port)
	} else {
		app, initial, portS, err = CreateApp(db, gh, repo, name, primary, port, command, dir, auto, appsBase)
	}
	if err != nil {
		return config.App{}, nil, "", err
	}
	if len(domains) == 0 {
		domains = initial
	}
	if _, err := MutateFleet(db, func(cp *config.Config) error {
		return AddAppConfig(cp, app, domains, portS)
	}); err != nil {
		return config.App{}, nil, "", err
	}
	return app, domains, portS, nil
}

// AddApp valida y persiste app+routes en una transacción.
func AddApp(db *store.DB, app config.App, domains []string, port string) error {
	_, err := MutateFleet(db, func(cfg *config.Config) error {
		return AddAppConfig(cfg, app, domains, port)
	})
	return err
}

// AddAppConfig valida y agrega app+rutas sobre la copia, sin persistir.
func AddAppConfig(cfg *config.Config, app config.App, domains []string, port string) error {
	for _, a := range cfg.Apps {
		if a.Name == app.Name {
			return fmt.Errorf("apps: %q ya existe", app.Name)
		}
		if port != "" && a.Env["PORT"] == port {
			return fmt.Errorf("apps: puerto %s ya usado por %q", port, a.Name)
		}
	}
	if app.RestartDelay == "" {
		app.RestartDelay = "5s"
	}
	if app.MinUptime == "" {
		app.MinUptime = "10s"
	}
	if app.Runtime == "" {
		app.Runtime = config.RuntimeManaged
	}
	if err := app.ValidateApp(len(cfg.Apps)); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	routes := cfg.Routes
	hosts := make([]string, 0, len(domains))
	for _, h := range domains {
		if port == "" {
			return fmt.Errorf("domain sin port")
		}
		hosts = append(hosts, h)
	}
	if len(hosts) > 0 {
		routes = append(routes, config.Route{
			Hosts: hosts, Target: "http://127.0.0.1:" + port,
		})
	}
	if err := config.ValidateRoutes(routes); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	cfg.Apps = append(cfg.Apps, app)
	cfg.Routes = routes
	return nil
}

// PatchEnv aplica set/unset atómico sobre el entorno global (appName "")
// o de una app. Un solo camino para API, CLI y MCP.
func PatchEnv(db *store.DB, appName string, set map[string]string, unset []string) error {
	if appName == "" {
		_, err := MutateInfra(db, func(cfg *config.Config) error {
			return ApplyEnvPatch(cfg, "", set, unset)
		})
		return err
	}
	_, err := MutateFleet(db, func(cfg *config.Config) error {
		return ApplyEnvPatch(cfg, appName, set, unset)
	})
	return err
}

// ApplyEnvPatch muta una copia y publica solo si todo valida.
func ApplyEnvPatch(cfg *config.Config, appName string, set map[string]string, unset []string) error {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range unset {
		if err := ValidateEnvKey(k); err != nil {
			return err
		}
	}
	for _, k := range keys {
		if err := ValidateEnvKey(k); err != nil {
			return err
		}
	}
	if appName == "" {
		work := cfg.Clone()
		if work.GlobalEnv == nil {
			work.GlobalEnv = map[string]string{}
		}
		for _, k := range unset {
			delete(work.GlobalEnv, k)
		}
		for _, k := range keys {
			if err := SetGlobalEnvValue(work, k, set[k]); err != nil {
				return err
			}
		}
		cfg.GlobalEnv = work.GlobalEnv
		return nil
	}
	work := cfg.Clone()
	app := FindApp(work, appName)
	if app == nil {
		return fmt.Errorf("app %q inexistente", appName)
	}
	if app.Env == nil {
		app.Env = map[string]string{}
	}
	for _, k := range unset {
		delete(app.Env, k)
	}
	for _, k := range keys {
		if err := SetEnvValue(work, appName, k, set[k]); err != nil {
			return err
		}
	}
	if err := app.ValidateApp(0); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	cfg.Apps = work.Apps
	return nil
}

// ValidateEnvKey nombre de variable canónico. PORT se gestiona con `port`.
func ValidateEnvKey(k string) error {
	if k == "" {
		return fmt.Errorf("clave vacía")
	}
	if k == "PORT" {
		return fmt.Errorf("PORT se cambia con el campo port, no como variable")
	}
	for i, c := range k {
		ok := c == '_' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9'
		if !ok {
			return fmt.Errorf("clave inválida %q: usa A-Z, 0-9 y _ (sin empezar por número)", k)
		}
	}
	return nil
}

// SetEnv escribe var expandida en la app (transaccional).
func SetEnv(db *store.DB, appName, key, rawVal string) error {
	_, err := MutateFleet(db, func(cfg *config.Config) error {
		return SetEnvValue(cfg, appName, key, rawVal)
	})
	return err
}

// SetEnvValue expande y aplica la variable sobre la copia, sin persistir.
// Único hogar de la expansión, también usado por API con su propio lock.
func SetEnvValue(cfg *config.Config, appName, key, rawVal string) error {
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
	return nil
}

// SetGlobalEnv escribe una global expandida (transaccional).
func SetGlobalEnv(db *store.DB, key, rawVal string) error {
	_, err := MutateInfra(db, func(cfg *config.Config) error {
		return SetGlobalEnvValue(cfg, key, rawVal)
	})
	return err
}

// SetGlobalEnvValue expande y aplica una global sin persistir.
func SetGlobalEnvValue(cfg *config.Config, key, rawVal string) error {
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
	return nil
}

// UnsetGlobalEnv borra una global (transaccional).
func UnsetGlobalEnv(db *store.DB, key string) error {
	_, err := MutateInfra(db, func(cfg *config.Config) error {
		UnsetGlobalEnvValue(cfg, key)
		return nil
	})
	return err
}

// UnsetGlobalEnvValue borra una global sin persistir.
func UnsetGlobalEnvValue(cfg *config.Config, key string) {
	delete(cfg.GlobalEnv, key)
}

// execOwn inyectable (test de fallo al resolver el binario).
var execOwn = os.Executable

// CreateApp arma App+domains+port con inferencia (Suggest+PresetBuild).
// Unico hogar (CLI wizard pregunta aparte; MCP y API la usan).
// Crea el dir. No escribe en DB (eso es AddApp).
func CreateApp(db *store.DB, gh *github.Manager, repo, name, domain string, port int, command, dir string, auto bool, appsBase string) (config.App, []string, string, error) {
	var sug *discover.Suggestion
	if repo != "" {
		var err error
		sug, err = discover.Suggest(db, gh, repo)
		if err != nil {
			return config.App{}, nil, "", err
		}
	} else if dir != "" {
		// directorio local: misma inferencia que un repo (preset, nombre, puerto)
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return config.App{}, nil, "", fmt.Errorf("directorio no existe: %s", dir)
		}
		var err error
		sug, err = discover.Suggest(db, gh, dir)
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
		if appsBase == "" {
			appsBase = platform.DefaultAppsDir()
		}
		dir = filepath.Join(appsBase, name)
	}
	portS := ""
	if port > 0 {
		portS = strconv.Itoa(port)
	} else if sug.Port > 0 {
		portS = strconv.Itoa(sug.Port)
	}
	if portS == "" {
		p, err := discover.FreePort(db)
		if err != nil {
			return config.App{}, nil, "", fmt.Errorf("sin puerto libre: %w", err)
		}
		portS = strconv.Itoa(p)
	}
	hosts := []string{}
	if domain != "" {
		hosts = []string{domain}
	} else {
		host, err := domains.Default(name, db.GetSetting(store.KeyBaseDomain), db.GetSetting(store.KeyPublicIP))
		if err != nil {
			return config.App{}, nil, "", err
		}
		hosts = []string{host}
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
		ref := sug.Branch
		if ref == "" {
			ref = "main"
		}
		app.Deploy = &config.Deploy{
			Repo: repo, Ref: ref, Build: build, Auto: auto,
		}
	}
	return app, hosts, portS, nil
}

// CreateExternalApp registra una app que solo usa el proxy: sin command,
// sin deploy, sin supervisor. El dev server lo corre el humano en Dir.
func CreateExternalApp(db *store.DB, name, domain, dir string, port int) (config.App, []string, string, error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return config.App{}, nil, "", fmt.Errorf("directorio no existe: %s", dir)
	}
	sug, err := discover.Suggest(db, nil, dir)
	if err != nil {
		return config.App{}, nil, "", err
	}
	if name == "" {
		name = sug.Name
	}
	if name == "" {
		return config.App{}, nil, "", fmt.Errorf("sin nombre (pasa name)")
	}
	portS := ""
	if port > 0 {
		portS = strconv.Itoa(port)
	} else if sug.Port > 0 {
		portS = strconv.Itoa(sug.Port)
	}
	if portS == "" {
		return config.App{}, nil, "", fmt.Errorf("sin puerto libre en port_pool")
	}
	hosts := []string{}
	if domain != "" {
		hosts = []string{domain}
	} else {
		host, err := domains.Default(name, db.GetSetting(store.KeyBaseDomain), db.GetSetting(store.KeyPublicIP))
		if err != nil {
			return config.App{}, nil, "", err
		}
		hosts = []string{host}
	}
	app := config.App{
		Name: name, Runtime: config.RuntimeExternal, Dir: dir,
		Env: map[string]string{"PORT": portS},
	}
	return app, hosts, portS, nil
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
