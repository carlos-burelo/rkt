// Comandos de gestión sobre SQLite (la flota vive en DB; el YAML solo
// conserva infra estable). Toda mutación valida y persiste atómico.
// routes/redirect aplican por hot-reload; apps/env exigen restart.
package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func jsonDecode(resp *http.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// merged abre DB + fusiona infra YAML + flota DB. Cerrar con db.Close().
func merged(cfgPath string) (*config.Config, *store.DB) {
	cfg, db, err := cfgfile.LoadMerged(cfgPath)
	if err != nil {
		_ = db.Close()
		log.Fatalf("config: %v", err)
	}
	return cfg, db
}

func persist(cfgPath string, db *store.DB, cfg *config.Config) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("inválido, no guardado: %v", err)
	}
	if err := db.ImportFleet(cfg); err != nil {
		log.Fatalf("guardar: %v", err)
	}
}

func cmdApps(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: apps <list|add|rm>")
	}
	switch args[0] {
	case "list":
		cfg, db := merged(cfgPath)
		defer db.Close()
		if len(cfg.Apps) == 0 {
			fmt.Println("(sin apps)")
			return
		}
		for _, a := range cfg.Apps {
			fmt.Printf("%-15s cmd=%s dir=%s\n", a.Name, a.Command, a.Dir)
		}
	case "add":
		appsAdd(args[1:], cfgPath)
	case "rm":
		if len(args) != 2 || args[1] == "" {
			log.Fatal("uso: apps rm <name>")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		kept := []config.App{}
		found := false
		for _, a := range cfg.Apps {
			if a.Name == args[1] {
				found = true
				continue
			}
			kept = append(kept, a)
		}
		if !found {
			log.Fatalf("apps: %q inexistente", args[1])
		}
		cfg.Apps = kept
		persist(cfgPath, db, cfg)
		fmt.Printf("app %s eliminada de DB (restart requerido). Revisa huérfanas: routes list\n", args[1])
	default:
		log.Fatalf("apps: subcomando %q (list|add|rm)", args[0])
	}
}

// appsAdd cablea app+repo+dominio+puerto de una vez:
//
//	rkt-proxy apps add --name app2 --repo https://github.com/o/app2.git \
//	  --domain app2.coatl.in --port 3002 --command C:\nodejs\node.exe \
//	  --preset next --auto
func appsAdd(args []string, cfgPath string) {
	fs := flag.NewFlagSet("apps add", flag.ContinueOnError)
	name := fs.String("name", "", "nombre [a-z0-9-]")
	repo := fs.String("repo", "", "repo git https (sin él, sin deploy)")
	ref := fs.String("ref", "main", "rama/tag/SHA")
	var domains multiFlag
	fs.Var(&domains, "domain", "dominio (repetible)")
	port := fs.String("port", "", "puerto local (con --domain)")
	dir := fs.String("dir", "", "dir app (default C:\\apps\\<name>)")
	command := fs.String("command", "", "binario absoluto")
	preset := fs.String("preset", "", "next|node|python")
	auto := fs.Bool("auto", false, "auto-deploy")
	var builds multiFlag
	fs.Var(&builds, "build", "paso build (repetible, gana a preset)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *name == "" || *command == "" {
		log.Fatal("apps add requiere --name y --command (ruta absoluta)")
	}
	if len(domains) > 0 && *port == "" {
		log.Fatal("apps add: --domain requiere --port")
	}
	if *dir == "" {
		if runtime.GOOS == "windows" {
			*dir = `C:\apps\` + *name
		} else {
			*dir = "/srv/" + *name
		}
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("apps add: crear dir: %v", err)
	}

	build := []string(builds)
	env := map[string]string{}
	appArgs := []string{}
	switch *preset {
	case "", "none":
	case "next":
		appArgs = []string{`.next\standalone\server.js`}
		if len(build) == 0 {
			build = []string{"npm ci --omit=dev", "npm run build"}
		}
		if *port != "" {
			env["PORT"] = *port
		}
	case "node":
		appArgs = []string{"server.js"}
		if len(build) == 0 {
			build = []string{"npm ci --omit=dev"}
		}
		if *port != "" {
			env["PORT"] = *port
		}
	case "python":
		appArgs = []string{"-u", "app.py"}
		if *port != "" {
			env["PORT"] = *port
		}
	default:
		log.Fatalf("preset %q (next|node|python)", *preset)
	}

	cfg, db := merged(cfgPath)
	defer db.Close()
	for _, a := range cfg.Apps {
		if a.Name == *name {
			log.Fatalf("apps: %q ya existe", *name)
		}
	}
	app := config.App{
		Name: *name, Command: *command, Args: appArgs,
		Dir: *dir, Env: env, RestartDelay: "5s",
	}
	if *repo != "" {
		app.Deploy = &config.Deploy{
			Repo: *repo, Ref: *ref, Build: build, Auto: *auto,
		}
	}
	cfg.Apps = append(cfg.Apps, app)
	for _, h := range domains {
		cfg.Routes = append(cfg.Routes, config.Route{
			Hosts: []string{h}, Target: "http://127.0.0.1:" + *port,
		})
	}
	persist(cfgPath, db, cfg)
	fmt.Printf("app %s en DB.", *name)
	if len(domains) > 0 {
		fmt.Printf(" route %s -> :%s (hot-reload).", domains, *port)
	}
	fmt.Print("\nPasos: sc.exe stop RktProxy; sc.exe start RktProxy; deploy " + *name + "\n")
}

func cmdRoutes(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: routes <list|add|rm>")
	}
	switch args[0] {
	case "list":
		cfg, db := merged(cfgPath)
		defer db.Close()
		if len(cfg.Routes) == 0 {
			fmt.Println("(sin routes)")
			return
		}
		for _, r := range cfg.Routes {
			fmt.Printf("%-40s -> %s\n", strings.Join(r.Hosts, ","), r.Target)
		}
	case "add":
		fs := flag.NewFlagSet("routes add", flag.ContinueOnError)
		var hosts multiFlag
		fs.Var(&hosts, "host", "host (repetible)")
		target := fs.String("target", "", "http://127.0.0.1:puerto")
		if err := fs.Parse(args[1:]); err != nil {
			os.Exit(2)
		}
		if len(hosts) == 0 || *target == "" {
			log.Fatal("routes add requiere --host (repetible) y --target")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		cfg.Routes = append(cfg.Routes, config.Route{Hosts: hosts, Target: *target})
		persist(cfgPath, db, cfg)
		fmt.Printf("route %s -> %s (hot-reload la aplica)\n", hosts, *target)
	case "rm":
		if len(args) != 2 || args[1] == "" {
			log.Fatal("uso: routes rm <host>")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		kept := []config.Route{}
		removed := false
		for _, r := range cfg.Routes {
			hosts := []string{}
			for _, h := range r.Hosts {
				if strings.EqualFold(h, args[1]) {
					removed = true
					continue
				}
				hosts = append(hosts, h)
			}
			if len(hosts) > 0 {
				r.Hosts = hosts
				kept = append(kept, r)
			} else if len(r.Hosts) > 0 {
				removed = true
			}
		}
		if !removed {
			log.Fatalf("routes: host %q no encontrado", args[1])
		}
		cfg.Routes = kept
		persist(cfgPath, db, cfg)
		fmt.Printf("host %s eliminado (hot-reload)\n", args[1])
	default:
		log.Fatalf("routes: subcomando %q (list|add|rm)", args[0])
	}
}

func cmdEnv(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: env <list|set|unset> ...")
	}
	switch args[0] {
	case "list":
		if len(args) != 2 {
			log.Fatal("uso: env list <app>")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		app := findApp(cfg, args[1])
		if len(app.Env) == 0 {
			fmt.Println("(sin env inline; puede haber env_file)")
			return
		}
		for k, v := range app.Env {
			fmt.Printf("%s=%s\n", k, v)
		}
	case "set":
		if len(args) != 3 {
			log.Fatal("uso: env set <app> CLAVE=valor")
		}
		k, v, ok := strings.Cut(args[2], "=")
		if !ok || strings.TrimSpace(k) == "" {
			log.Fatalf("env set: %q no es CLAVE=valor", args[2])
		}
		k = strings.TrimSpace(k)
		cfg, db := merged(cfgPath)
		defer db.Close()
		a := findAppPtr(cfg, args[1])
		if a.Env == nil {
			a.Env = map[string]string{}
		}
		// expande solo el valor nuevo (el resto ya está expandido en DB)
		scope := dotenv.EnvironMap()
		for ek, ev := range a.Env {
			if ek != k {
				scope[ek] = ev
			}
		}
		if a.EnvFile != "" {
			if raw, err := dotenv.ParseFile(a.EnvFile); err == nil {
				for fk, fv := range dotenv.Expand(raw, dotenv.EnvironMap()) {
					if _, exists := scope[fk]; !exists {
						scope[fk] = fv
					}
				}
			}
		}
		a.Env[k] = dotenv.Expand(map[string]string{k: v}, scope)[k]
		persist(cfgPath, db, cfg)
		fmt.Printf("%s=%s (restart requerido)\n", k, v)
	case "unset":
		if len(args) != 3 {
			log.Fatal("uso: env unset <app> CLAVE")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		a := findAppPtr(cfg, args[1])
		delete(a.Env, args[2])
		persist(cfgPath, db, cfg)
		fmt.Printf("%s eliminada (restart requerido)\n", args[2])
	default:
		log.Fatalf("env: subcomando %q (list|set|unset)", args[0])
	}
}

func findApp(cfg *config.Config, name string) config.App {
	for _, a := range cfg.Apps {
		if a.Name == name {
			return a
		}
	}
	log.Fatalf("app %q inexistente", name)
	return config.App{}
}

func findAppPtr(cfg *config.Config, name string) *config.App {
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == name {
			return &cfg.Apps[i]
		}
	}
	log.Fatalf("app %q inexistente", name)
	return nil
}

func cmdSettings(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: settings <list|set>")
	}
	_, db := merged(cfgPath)
	defer db.Close()
	switch args[0] {
	case "list":
		for _, k := range []string{"node_bin", "python_bin", "base_domain", "port_pool"} {
			fmt.Printf("%s=%s\n", k, db.GetSetting(k))
		}
	case "set":
		if len(args) != 2 {
			log.Fatal("uso: settings set CLAVE=valor")
		}
		k, v, ok := strings.Cut(args[1], "=")
		if !ok || strings.TrimSpace(k) == "" {
			log.Fatalf("settings set: %q no es CLAVE=valor", args[1])
		}
		if err := db.SetSetting(strings.TrimSpace(k), v); err != nil {
			log.Fatalf("settings: %v", err)
		}
		fmt.Printf("%s=%s\n", strings.TrimSpace(k), v)
	default:
		log.Fatalf("settings: subcomando %q (list|set)", args[0])
	}
}

func cmdConfig(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: config <export|import>")
	}
	switch args[0] {
	case "export":
		cmdExport(args[1:], cfgPath)
	case "import":
		cmdImport(args[1:], cfgPath)
	default:
		log.Fatalf("config: subcomando %q (export|import)", args[0])
	}
}

func cmdExport(args []string, cfgPath string) {
	out := cfgPath + ".export.yaml"
	if len(args) > 1 {
		log.Fatal("uso: config export [salida]")
	}
	if len(args) == 1 {
		out = args[0]
	}
	cfg, db := merged(cfgPath)
	defer db.Close()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatalf("export: %v", err)
	}
	// settings en bloque aparte para backup completo
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		log.Fatalf("export: %v", err)
	}
	settings := map[string]string{}
	for _, k := range []string{"node_bin", "python_bin", "base_domain", "port_pool"} {
		if v := db.GetSetting(k); v != "" {
			settings[k] = v
		}
	}
	if len(settings) > 0 {
		var settingsNode yaml.Node
		settingsNode.Encode(settings)
		doc.Content[0].Content = append(doc.Content[0].Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x-settings"},
			&settingsNode)
		var err error
		data, err = yaml.Marshal(&doc)
		if err != nil {
			log.Fatalf("export: %v", err)
		}
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		log.Fatalf("export: %v", err)
	}
	fmt.Printf("exportado a %s (respaldo en git)\n", out)
}

func cmdImport(args []string, cfgPath string) {
	if len(args) != 1 {
		log.Fatal("uso: config import <archivo>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("import: %v", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("import: %v", err)
	}
	if err := cfg.ResolveEnvAll(); err != nil {
		log.Fatalf("import: %v", err)
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	defer db.Close()
	if err := db.ImportFleet(&cfg); err != nil {
		log.Fatalf("import: %v", err)
	}
	var doc struct {
		Settings map[string]string `yaml:"x-settings"`
	}
	if err := yaml.Unmarshal(data, &doc); err == nil {
		for k, v := range doc.Settings {
			if err := db.SetSetting(k, v); err != nil {
				log.Fatalf("import: %v", err)
			}
		}
	}
	fmt.Println("flota importada (restart requerido para apps)")
}

func cmdStatus(cfgPath string) {
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		log.Fatalf("status: %v", err)
	}
	if !cfg.Healthz.Enabled {
		log.Fatal("status: healthz desactivado")
	}
	resp, err := http.Get("http://" + cfg.Healthz.Listen + "/healthz")
	if err != nil {
		log.Fatalf("status: servicio no responde: %v", err)
	}
	defer resp.Body.Close()
	var st struct {
		Status   string `json:"status"`
		Version  string `json:"version"`
		Backends []struct {
			Target string `json:"target"`
			OK     bool   `json:"ok"`
		} `json:"backends"`
		Apps []struct {
			Name     string `json:"name"`
			Running  bool   `json:"running"`
			Pid      int    `json:"pid"`
			Restarts int    `json:"restarts"`
			Failed   bool   `json:"failed"`
		} `json:"apps"`
	}
	if err := jsonDecode(resp, &st); err != nil {
		log.Fatalf("status: %v", err)
	}
	fmt.Printf("rkt %s status=%s\n", st.Version, st.Status)
	for _, b := range st.Backends {
		fmt.Printf("  backend %-25s ok=%v\n", b.Target, b.OK)
	}
	for _, a := range st.Apps {
		fmt.Printf("  app %-15s running=%v pid=%d restarts=%d failed=%v\n",
			a.Name, a.Running, a.Pid, a.Restarts, a.Failed)
	}
}

func cmdRollback(args []string, cfgPath string) {
	if len(args) != 1 || args[0] == "" {
		log.Fatal("uso: rollback <app>")
	}
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		log.Fatalf("rollback: %v", err)
	}
	if !cfg.Healthz.Enabled {
		log.Fatal("rollback: requiere healthz activado")
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Post("http://"+cfg.Healthz.Listen+"/rollback/"+args[0], "application/json", nil)
	if err != nil {
		log.Fatalf("rollback: sin contacto: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Rolledback string `json:"rolledback"`
		Error      string `json:"error"`
	}
	_ = jsonDecode(resp, &out)
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("rollback: %s", out.Error)
	}
	fmt.Printf("rollback ok: %s (versión previa sirviendo)\n", out.Rolledback)
}

func cmdGithub(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: github <login|init>")
	}
	switch args[0] {
	case "login":
		cmdGithubLogin(cfgPath)
	case "init":
		cmdGithubInit(args[1:])
	default:
		log.Fatalf("github: subcomando %q (login|init)", args[0])
	}
}

func cmdGithubLogin(cfgPath string) {
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		log.Fatalf("github: %v", err)
	}
	if cfg.Github == nil || cfg.Github.AppSlug == "" {
		log.Fatal("github: configura github.app_slug en el YAML")
	}
	url := "https://github.com/apps/" + cfg.Github.AppSlug + "/installations/new"
	fmt.Println("Abre en el browser para vincular (instala en tus orgs/repos):")
	fmt.Println("  " + url)
	fmt.Println("Al aprobar, GitHub vuelve a /_rkt/setup y rkt registra solo.")
	fmt.Println("La App necesita Setup URL: https://TU-DOMINIO/_rkt/setup")
	openBrowser(url)
}

// cmdGithubInit guía crear la App vía manifest oficial:
//
//	rkt-proxy github init --name rkt-deployer --domain tu-dominio.coatl.in [--org dacyti-gsi]
//
// Imprime (y abre) la URL del form que crea la App en GitHub; al aprobar,
// GitHub vuelve a /_rkt/setup y rkt se autoconfigura (PEM + settings).
func cmdGithubInit(args []string) {
	fs := flag.NewFlagSet("github init", flag.ContinueOnError)
	name := fs.String("name", "rkt-deployer", "nombre de la App")
	domain := fs.String("domain", "", "dominio público del VPS")
	org := fs.String("org", "", "org GitHub (vacío = personal)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *name == "" || *domain == "" {
		log.Fatal("github init requiere --name y --domain")
	}
	link := githubInitURL(*domain, *name, *org)
	fmt.Println("Crea la GitHub App (permisos mínimos ya rellenados):")
	fmt.Println("  " + link)
	fmt.Println("Pulsa Create en GitHub; rkt se configura solo y te da el link de instalación.")
	openBrowser(link)
}

// githubInitURL construye la URL del form new-app.
func githubInitURL(domain, name, org string) string {
	link := "https://" + domain + "/_rkt/new-app?name=" + url.QueryEscape(name) + "&domain=" + url.QueryEscape(domain)
	if org != "" {
		link += "&org=" + url.QueryEscape(org)
	}
	return link
}

// openBrowser abre URL (best effort). En Windows, start exige título
// vacío antes de la URL entrecomillada; sin comillas, el & la parte.
func openBrowser(link string) {
	if runtime.GOOS != "windows" {
		return
	}
	_ = exec.Command("cmd.exe", "/C", "start", "", link).Start()
}

func cmdDeployments(args []string, cfgPath string) {
	n := 20
	app := ""
	// acepta `deployments list` además de `deployments` pelado
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n", "--lines":
			i++
			if i >= len(args) {
				log.Fatal("deployments: -n requiere número")
			}
			if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil || n < 1 {
				log.Fatalf("deployments: -n inválido %q", args[i])
			}
		case "--app":
			i++
			if i >= len(args) {
				log.Fatal("deployments: --app requiere nombre")
			}
			app = args[i]
		default:
			log.Fatalf("deployments: flag desconocido %q (uso: deployments [-n 20] [--app nombre])", args[i])
		}
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatalf("deployments: %v", err)
	}
	defer db.Close()
	deps, err := db.ListDeployments(app, n)
	if err != nil {
		log.Fatalf("deployments: %v", err)
	}
	if len(deps) == 0 {
		fmt.Println("(sin historial)")
		return
	}
	for _, d := range deps {
		fmt.Printf("#%-4d %-12s %-9s rev=%.7s %s\n", d.ID, d.App, d.Status, d.Rev, d.StartedAt)
	}
}
