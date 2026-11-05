// Comandos de gestión sobre SQLite (la flota vive en DB; el YAML solo
// conserva infra estable). Toda mutación valida y persiste atómico.
// routes/redirect aplican por hot-reload; apps/env exigen restart.
package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/term"

	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/hosts"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func jsonDecode(resp *http.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// wantJSON separa --json global de los args (para scripting).
func wantJSON(args []string) ([]string, bool) {
	rest := []string{}
	json := false
	for _, a := range args {
		if a == "--json" {
			json = true
			continue
		}
		rest = append(rest, a)
	}
	if *JSON {
		json = true
	}
	return rest, json
}

// merged abre DB + fusiona infra YAML + flota DB. Cerrar con db.Close().
func merged(cfgPath string) (*config.Config, *store.DB) {
	cfg, db, err := cfgfile.LoadMerged(cfgPath)
	if err != nil {
		if db != nil {
			db.Close()
		}
		log.Fatalf("config: %v", err)
	}
	return cfg, db
}

func persist(cfgPath string, db *store.DB, cfg *config.Config) {
	if err := cfgfile.SaveFleet(db, cfg); err != nil {
		log.Fatalf("guardar: %v", err)
	}
}

// prompt pregunta en terminal (Enter = default). Sin stdin, default.
func prompt(label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		fmt.Println()
		return def
	}
	if v := strings.TrimSpace(line); v != "" {
		return v
	}
	return def
}

// promptPassword lee sin eco (mín 12, pide dos veces). Sin consola
// (pipe/tests), cae a lectura visible con un solo reader (dos
// readers se roban el buffer y ciclan).
func promptPassword() string {
	fd := int(os.Stdin.Fd())
	var fb *bufio.Reader
	read := func(label string) string {
		fmt.Print(label)
		if fb == nil {
			if b, err := term.ReadPassword(fd); err == nil {
				fmt.Println()
				return string(b)
			}
			fb = bufio.NewReader(os.Stdin)
		}
		line, _ := fb.ReadString('\n')
		return strings.TrimSpace(line)
	}
	for {
		b1 := read("password (mín 12): ")
		b2 := read("repite password: ")
		if b1 != b2 {
			fmt.Println("no coinciden, de nuevo")
			continue
		}
		if len(b1) < 12 {
			fmt.Println("mínimo 12 caracteres")
			continue
		}
		return b1
	}
}

// cmdUsers alta/baja/lista/password (bootstrap: primer admin por aquí).
func cmdUsers(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: users <list|add|del|passwd>")
	}
	_, db := merged(cfgPath)
	defer db.Close()
	switch args[0] {
	case "list":
		users, err := db.ListUsers()
		if err != nil {
			log.Fatalf("users: %v", err)
		}
		if len(users) == 0 {
			fmt.Println("(sin usuarios; crea el primero: users add <nombre> --role admin)")
			return
		}
		for _, u := range users {
			fmt.Printf("%s\t%s\t%s\n", u.Name, u.Role, u.CreatedAt)
		}
	case "add":
		role, rest := "viewer", []string{}
		for i := 1; i < len(args); i++ {
			if args[i] == "--role" && i+1 < len(args) {
				role = args[i+1]
				i++
			} else if strings.HasPrefix(args[i], "--role=") {
				role = strings.TrimPrefix(args[i], "--role=")
			} else {
				rest = append(rest, args[i])
			}
		}
		if len(rest) != 1 {
			log.Fatal("uso: users add <nombre> [--role admin]")
		}
		name := rest[0]
		n, err := db.CountUsers()
		if err != nil {
			log.Fatalf("users: %v", err)
		}
		if n == 0 && role != store.RoleAdmin {
			fmt.Println("primer usuario debe ser admin (forzando --role admin)")
			role = store.RoleAdmin
		}
		if err := db.CreateUser(name, promptPassword(), role); err != nil {
			log.Fatalf("users add: %v", err)
		}
		fmt.Printf("usuario %s (%s) creado\n", name, role)
	case "del":
		if len(args) != 2 {
			log.Fatal("uso: users del <nombre>")
		}
		if err := db.DeleteUser(args[1]); err != nil {
			log.Fatalf("users del: %v", err)
		}
		fmt.Printf("usuario %s borrado\n", args[1])
	case "passwd":
		if len(args) != 2 {
			log.Fatal("uso: users passwd <nombre>")
		}
		if err := db.SetPassword(args[1], promptPassword()); err != nil {
			log.Fatalf("users passwd: %v", err)
		}
		fmt.Printf("password de %s actualizado (sesiones invalidadas)\n", args[1])
	default:
		log.Fatalf("users: subcomando %q (list|add|del|passwd)", args[0])
	}
}

// cmdHosts gestiona el bloque de rkt en el archivo hosts (DNS local).
func cmdHosts(args []string) {
	if len(args) == 0 {
		log.Fatal("uso: hosts list|add|rm <host>")
	}
	switch args[0] {
	case "list":
		list, err := hosts.List()
		if err != nil {
			log.Fatalf("hosts list: %v", err)
		}
		for _, h := range list {
			fmt.Println(h)
		}
		if len(list) == 0 {
			fmt.Println("(sin hosts de rkt)")
		}
		fmt.Println("# archivo:", hosts.Path())
	case "add":
		if len(args) != 2 {
			log.Fatal("uso: hosts add <host>")
		}
		if err := hosts.Add(args[1]); err != nil {
			log.Fatalf("hosts add: %v (ejecuta como administrador)", err)
		}
		fmt.Printf("%s → 127.0.0.1\n", args[1])
	case "rm":
		if len(args) != 2 {
			log.Fatal("uso: hosts rm <host>")
		}
		if err := hosts.Remove(args[1]); err != nil {
			log.Fatalf("hosts rm: %v (ejecuta como administrador)", err)
		}
		fmt.Printf("%s quitado\n", args[1])
	default:
		log.Fatalf("hosts: subcomando %q (list|add|rm)", args[0])
	}
}

func cmdApps(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: apps <list|add|rm>")
	}
	switch args[0] {
	case "list":
		_, asJSON := wantJSON(args[1:])
		cfg, db := merged(cfgPath)
		defer db.Close()
		type appOut struct {
			Name    string `json:"name"`
			Command string `json:"command"`
			Dir     string `json:"dir"`
		}
		out := []appOut{}
		for _, a := range cfg.Apps {
			out = append(out, appOut{a.Name, a.Command, a.Dir})
		}
		if asJSON {
			if out == nil {
				out = []appOut{}
			}
			json.NewEncoder(os.Stdout).Encode(out)
			return
		}
		if len(out) == 0 {
			fmt.Println("(sin apps)")
			return
		}
		for _, a := range out {
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
		dropped, err := cfgfile.RemoveApp(cfg, args[1], false)
		if err != nil {
			log.Fatalf("apps: %v", err)
		}
		persist(cfgPath, db, cfg)
		fmt.Printf("app %s eliminada de DB (restart requerido), %d rutas retiradas\n", args[1], dropped)
	default:
		log.Fatalf("apps: subcomando %q (list|add|rm)", args[0])
	}
}

// appsAdd cablea app+repo+dominio+puerto. Flags dados valen; lo que falte
// se pregunta con defaults inferidos (Enter acepta):
//
//	rkt apps add https://github.com/o/app2.git
//	rkt apps add --name app2 --repo ... --domain app2.coatl.in --port 3002 \
//	  --command C:\nodejs\node.exe --auto
func appsAdd(args []string, cfgPath string) {
	fs := flag.NewFlagSet("apps add", flag.ContinueOnError)
	name := fs.String("name", "", "nombre [a-z0-9-]")
	repo := fs.String("repo", "", "repo git o dir local (o primer posicional)")
	ref := fs.String("ref", "main", "rama/tag/SHA")
	var domains multiFlag
	fs.Var(&domains, "domain", "dominio (repetible)")
	port := fs.String("port", "", "puerto local (con --domain)")
	dir := fs.String("dir", "", "dir app")
	command := fs.String("command", "", "binario absoluto (anula el plan)")
	external := fs.Bool("external", false, "app externa: solo proxy, sin proceso (dev server aparte)")
	auto := fs.Bool("auto", false, "auto-deploy")
	var builds multiFlag
	fs.Var(&builds, "build", "paso build (repetible, anula el plan)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *repo == "" && fs.NArg() > 0 {
		*repo = fs.Arg(0)
	}

	cfg, db := merged(cfgPath)
	defer db.Close()
	if *external {
		if *name == "" || *dir == "" || len(domains) == 0 {
			log.Fatal("apps add --external requiere --name, --dir y --domain")
		}
		p := 0
		if *port != "" {
			p, _ = strconv.Atoi(*port)
		}
		app, doms, portS, err := cfgfile.CreateExternalApp(db, *name, domains[0], *dir, p)
		if err != nil {
			log.Fatalf("apps add: %v", err)
		}
		if err := cfgfile.AddApp(db, app, doms, portS); err != nil {
			log.Fatalf("apps add: %v", err)
		}
		fmt.Printf("app externa %s → %s puerto %s (sin proceso; corre: pnpm dev)\n", app.Name, strings.Join(doms, ","), portS)
		return
	}
	var sug *discover.Suggestion
	if *repo != "" {
		if s, err := discover.Suggest(db, nil, *repo); err == nil {
			sug = s
		}
	}

	nm := *name
	if nm == "" {
		d := ""
		if sug != nil {
			d = sug.Name
		}
		nm = prompt("nombre", d)
		if nm == "" {
			log.Fatal("apps add requiere --name (o repo del que inferirlo)")
		}
	}
	for _, a := range cfg.Apps {
		if a.Name == nm {
			log.Fatalf("apps: %q ya existe", nm)
		}
	}
	dd := *dir
	if dd == "" {
		if sug != nil && sug.Dir != "" {
			dd = sug.Dir
		} else {
			dd = `C:\apps\` + nm
		}
		dd = prompt("dir app", dd)
	}
	if err := os.MkdirAll(dd, 0o755); err != nil {
		log.Fatalf("apps add: crear dir: %v", err)
	}
	// Plan canónico: si hay dir/repo con señales, decide arranque y build.
	var plan buildpack.Plan
	if sug != nil {
		plan = sug.Plan
	}
	if plan.Kind == "" {
		if p, err := buildpack.DetectDir(dd); err == nil {
			plan = p
		}
	}
	cmd := *command
	if cmd == "" && plan.IsStatic() {
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("apps add: binario propio: %v", err)
		}
		cmd = exe
	}
	if cmd == "" && len(plan.Start) > 0 {
		cmd = plan.Start[0]
	}
	if cmd == "" {
		d := ""
		if sug != nil {
			d = sug.Command
		}
		cmd = prompt("binario absoluto", d)
		if cmd == "" {
			log.Fatal("apps add: sin plan de build (pasa --command o un dir con package.json/go.mod/index.html)")
		}
	}
	pt := *port
	if pt == "" && sug != nil && sug.Port > 0 {
		pt = prompt("puerto local", strconv.Itoa(sug.Port))
	}
	if len(domains) == 0 {
		d := ""
		if sug != nil {
			d = sug.Domain
		}
		if ans := prompt("dominios (coma, vacío = sin route)", d); ans != "" {
			for _, h := range strings.Split(ans, ",") {
				if h = strings.TrimSpace(h); h != "" {
					domains = append(domains, h)
				}
			}
		}
	}
	if len(domains) > 0 && pt == "" {
		pt = prompt("puerto local", "3000")
	}
	rp := *repo
	if rp == "" {
		rp = prompt("repo git (vacío = sin deploy)", "")
	}
	rf, au := *ref, *auto
	if rp != "" && *ref == "main" && !*auto {
		// solo pregunta si no vino explícito... simplifica: defaults
		rf = prompt("ref rama/tag/SHA", *ref)
		au = promptBool("auto-deploy", *auto)
	}

	build := []string(builds)
	if len(build) == 0 {
		build = append(append([]string{}, plan.Install...), plan.Build...)
	}
	env := map[string]string{}
	if pt != "" {
		env["PORT"] = pt
	}
	appArgs := []string{}
	staticDir := ""
	if *command == "" {
		if plan.IsStatic() {
			staticDir = plan.Static(dd)
			appArgs = []string{"__static", staticDir}
		} else if len(plan.Start) > 1 {
			appArgs = append(appArgs, plan.Start[1:]...)
		}
	}

	app := config.App{
		Name: nm, Command: cmd, Args: appArgs, Static: staticDir,
		Dir: dd, Env: env, RestartDelay: "5s",
	}
	if rp != "" {
		app.Deploy = &config.Deploy{
			Repo: rp, Ref: rf, Build: build, Auto: au,
		}
	}
	if err := cfgfile.AddApp(db, app, domains, pt); err != nil {
		log.Fatalf("apps add: %v", err)
	}
	fmt.Printf("app %s en DB.", nm)
	if len(domains) > 0 {
		fmt.Printf(" route %s -> :%s (hot-reload).", domains, pt)
	}
	fmt.Print("\nPasos: sc.exe stop Rkt; sc.exe start Rkt; deploy " + nm + "\n")
}

// promptBool pregunta s/n.
func promptBool(label string, def bool) bool {
	d := "n"
	if def {
		d = "s"
	}
	ans := strings.ToLower(prompt(label+" s/n", d))
	return ans == "s" || ans == "sí" || ans == "si" || ans == "y" || ans == "yes" || ans == "1"
}

func cmdRoutes(args []string, cfgPath string) {
	if len(args) == 0 {
		log.Fatal("uso: routes <list|add|rm>")
	}
	switch args[0] {
	case "list":
		_, asJSON := wantJSON(args[1:])
		cfg, db := merged(cfgPath)
		defer db.Close()
		type routeOut struct {
			Hosts  []string `json:"hosts"`
			Target string   `json:"target"`
		}
		out := []routeOut{}
		for _, r := range cfg.Routes {
			out = append(out, routeOut{r.Hosts, r.Target})
		}
		if asJSON {
			if out == nil {
				out = []routeOut{}
			}
			json.NewEncoder(os.Stdout).Encode(out)
			return
		}
		if len(out) == 0 {
			fmt.Println("(sin routes)")
			return
		}
		for _, r := range out {
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
		if err := cfgfile.AddRoute(cfg, hosts, *target); err != nil {
			log.Fatalf("routes add: %v", err)
		}
		persist(cfgPath, db, cfg)
		fmt.Printf("route %s -> %s (hot-reload la aplica)\n", hosts, *target)
	case "rm":
		if len(args) != 2 || args[1] == "" {
			log.Fatal("uso: routes rm <host>")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		removed, err := cfgfile.RemoveRouteHost(cfg, args[1])
		if err != nil {
			log.Fatalf("routes rm: %v", err)
		}
		if !removed {
			log.Fatalf("routes: host %q no encontrado", args[1])
		}
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
		cfg, db := merged(cfgPath)
		defer db.Close()
		if err := cfgfile.SetEnv(db, cfg, args[1], strings.TrimSpace(k), v); err != nil {
			log.Fatalf("env set: %v", err)
		}
		fmt.Printf("%s=%s (restart requerido)\n", strings.TrimSpace(k), v)
	case "unset":
		if len(args) != 3 {
			log.Fatal("uso: env unset <app> CLAVE")
		}
		cfg, db := merged(cfgPath)
		defer db.Close()
		if err := cfgfile.UnsetEnv(cfg, args[1], args[2]); err != nil {
			log.Fatalf("env unset: %v", err)
		}
		persist(cfgPath, db, cfg)
		fmt.Printf("%s eliminada (restart requerido)\n", args[2])
	case "global":
		cfg, db := merged(cfgPath)
		defer db.Close()
		if len(args) == 1 {
			if len(cfg.GlobalEnv) == 0 {
				fmt.Println("(sin variables globales)")
				return
			}
			keys := make([]string, 0, len(cfg.GlobalEnv))
			for k := range cfg.GlobalEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("%s=%s\n", k, cfg.GlobalEnv[k])
			}
			return
		}
		switch args[1] {
		case "set":
			if len(args) != 3 {
				log.Fatal("uso: env global set CLAVE=valor")
			}
			k, v, ok := strings.Cut(args[2], "=")
			if !ok || strings.TrimSpace(k) == "" {
				log.Fatalf("env global set: %q no es CLAVE=valor", args[2])
			}
			if err := cfgfile.SetGlobalEnv(db, cfg, strings.TrimSpace(k), v); err != nil {
				log.Fatalf("env global set: %v", err)
			}
			fmt.Printf("%s=%s global (las apps la heredan al reiniciar)\n", strings.TrimSpace(k), v)
		case "unset":
			if len(args) != 3 {
				log.Fatal("uso: env global unset CLAVE")
			}
			if err := cfgfile.UnsetGlobalEnv(db, cfg, args[2]); err != nil {
				log.Fatalf("env global unset: %v", err)
			}
			fmt.Printf("%s global eliminada\n", args[2])
		default:
			log.Fatalf("env global: subcomando %q (set|unset)", args[1])
		}
	default:
		log.Fatalf("env: subcomando %q (list|set|unset|global)", args[0])
	}
}

func findApp(cfg *config.Config, name string) config.App {
	if a := cfgfile.FindApp(cfg, name); a != nil {
		return *a
	}
	log.Fatalf("app %q inexistente", name)
	return config.App{}
}

func findAppPtr(cfg *config.Config, name string) *config.App {
	if a := cfgfile.FindApp(cfg, name); a != nil {
		return a
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
		for _, k := range store.KnownSettings() {
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
	out := cfgPath + ".export.json"
	if len(args) > 1 {
		log.Fatal("uso: config export [salida]")
	}
	if len(args) == 1 {
		out = args[0]
	}
	cfg, db := merged(cfgPath)
	defer db.Close()
	settings := map[string]string{}
	for _, k := range store.KnownSettings() {
		if v := db.GetSetting(k); v != "" {
			settings[k] = v
		}
	}
	data, err := config.ExportJSON(cfg, settings)
	if err != nil {
		log.Fatalf("export: %v", err)
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
	cfg, settings, err := config.ImportJSON(data)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	defer db.Close()
	if err := db.ImportFleet(cfg); err != nil {
		log.Fatalf("import: %v", err)
	}
	if err := cfgfile.SaveInfra(db, cfg); err != nil {
		log.Fatalf("import: %v", err)
	}
	for k, v := range settings {
		if err := db.SetSetting(k, v); err != nil {
			log.Fatalf("import: %v", err)
		}
	}
	fmt.Println("flota importada (restart requerido para apps)")
}

func cmdStatus(args []string, cfgPath string) {
	_, asJSON := wantJSON(args)
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
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(st)
		return
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
	client := &http.Client{Timeout: deployTimeout}
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

// openBrowser abre URL (best effort) con rundll32: la URL va como argumento
// directo y ningún shell la parte.
func openBrowser(link string) {
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", link).Start()
}

func cmdDeployments(args []string, cfgPath string) {
	n := 20
	app := ""
	// acepta `deployments list` además de `deployments` pelado
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	args, asJSON := wantJSON(args)
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
	if len(deps) == 0 && !asJSON {
		fmt.Println("(sin historial)")
		return
	}
	if asJSON {
		if deps == nil {
			deps = []store.Deployment{}
		}
		json.NewEncoder(os.Stdout).Encode(deps)
		return
	}
	for _, d := range deps {
		by := ""
		if d.TriggeredBy != "" {
			by = " por " + d.TriggeredBy
		}
		fmt.Printf("#%-4d %-12s %-9s rev=%.7s %s%s\n", d.ID, d.App, d.Status, d.Rev, d.StartedAt, by)
	}
}
