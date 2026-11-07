// Comandos de gestión sobre SQLite (flota e infra viven en DB).
// Toda mutación valida y persiste atómico.
// routes/redirect aplican por hot-reload; apps/env exigen restart.
package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/discover"
	domainx "rkt-proxy/internal/domains"
	"rkt-proxy/internal/hosts"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/platform"
	"rkt-proxy/internal/settings"
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

// merged abre DB + fusiona infra y flota. Cerrar con db.Close().
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
// oneLineStdin lee una línea (para --password-stdin del instalador).
func oneLineStdin() string {
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

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
		role, rest, stdinPass := "viewer", []string{}, false
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--role" && i+1 < len(args):
				role = args[i+1]
				i++
			case strings.HasPrefix(args[i], "--role="):
				role = strings.TrimPrefix(args[i], "--role=")
			case args[i] == "--password-stdin":
				stdinPass = true
			default:
				rest = append(rest, args[i])
			}
		}
		if len(rest) != 1 {
			log.Fatal("uso: users add <nombre> [--role admin] [--password-stdin]")
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
		pass := ""
		if stdinPass {
			pass = oneLineStdin()
			if len(pass) < 12 {
				log.Fatal("users add: password de stdin demasiado corto (mín 12)")
			}
		} else {
			pass = promptPassword()
		}
		if err := db.CreateUser(name, pass, role); err != nil {
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
		rest, stdinPass := []string{}, false
		for _, a := range args[1:] {
			if a == "--password-stdin" {
				stdinPass = true
				continue
			}
			rest = append(rest, a)
		}
		if len(rest) != 1 {
			log.Fatal("uso: users passwd <nombre> [--password-stdin]")
		}
		pass := ""
		if stdinPass {
			pass = oneLineStdin()
			if len(pass) < 12 {
				log.Fatal("users passwd: password de stdin demasiado corto (mín 12)")
			}
		} else {
			pass = promptPassword()
		}
		if err := db.SetPassword(rest[0], pass); err != nil {
			log.Fatalf("users passwd: %v", err)
		}
		fmt.Printf("password de %s actualizado (sesiones invalidadas)\n", rest[0])
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
		_, db := merged(cfgPath)
		defer db.Close()
		dropped := 0
		if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
			n, err := cfgfile.RemoveApp(cp, args[1], false)
			dropped = n
			return err
		}); err != nil {
			log.Fatalf("apps: %v", err)
		}
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
		if *name == "" || *dir == "" {
			log.Fatal("apps add --external requiere --name y --dir")
		}
		p := 0
		if *port != "" {
			p, _ = strconv.Atoi(*port)
		}
		primary := ""
		if len(domains) > 0 {
			primary = domains[0]
		}
		app, initial, portS, err := cfgfile.CreateExternalApp(db, *name, primary, *dir, p)
		if err != nil {
			log.Fatalf("apps add: %v", err)
		}
		if len(domains) == 0 {
			domains = initial
		}
		if err := cfgfile.AddApp(db, app, domains, portS); err != nil {
			log.Fatalf("apps add: %v", err)
		}
		fmt.Printf("app externa %s → %s puerto %s (sin proceso; corre: pnpm dev)\n", app.Name, strings.Join(domains, ","), portS)
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
			dd = filepath.Join(platform.DefaultAppsDir(), nm)
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
		base := db.GetSetting(store.KeyBaseDomain)
		publicIP := db.GetSetting(store.KeyPublicIP)
		d, _ := domainx.Default(nm, base, publicIP)
		label := "dominios (coma, vacío = dominio local por defecto)"
		if domainx.RequiresDomain(publicIP, base) {
			label = "dominio con acceso (obligatorio en servidor público)"
		}
		ans := prompt(label, d)
		if ans == "" && domainx.RequiresDomain(publicIP, base) {
			log.Fatal("apps add: servidor público sin dominio; pasa --domain o configura base_domain")
		}
		if ans != "" {
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
		_, db := merged(cfgPath)
		defer db.Close()
		if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
			return cfgfile.AddRoute(cp, hosts, *target)
		}); err != nil {
			log.Fatalf("routes add: %v", err)
		}
		fmt.Printf("route %s -> %s (hot-reload la aplica)\n", hosts, *target)
	case "rm":
		if len(args) != 2 || args[1] == "" {
			log.Fatal("uso: routes rm <host>")
		}
		_, db := merged(cfgPath)
		defer db.Close()
		removed := false
		if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
			ok, err := cfgfile.RemoveRouteHost(cp, args[1])
			removed = ok
			return err
		}); err != nil {
			log.Fatalf("routes rm: %v", err)
		}
		if !removed {
			log.Fatalf("routes: host %q no encontrado", args[1])
		}
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
		_, db := merged(cfgPath)
		defer db.Close()
		if err := cfgfile.SetEnv(db, args[1], strings.TrimSpace(k), v); err != nil {
			log.Fatalf("env set: %v", err)
		}
		fmt.Printf("%s=%s (restart requerido)\n", strings.TrimSpace(k), v)
	case "unset":
		if len(args) != 3 {
			log.Fatal("uso: env unset <app> CLAVE")
		}
		_, db := merged(cfgPath)
		defer db.Close()
		if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
			return cfgfile.UnsetEnv(cp, args[1], args[2])
		}); err != nil {
			log.Fatalf("env unset: %v", err)
		}
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
			if err := cfgfile.SetGlobalEnv(db, strings.TrimSpace(k), v); err != nil {
				log.Fatalf("env global set: %v", err)
			}
			fmt.Printf("%s=%s global (las apps la heredan al reiniciar)\n", strings.TrimSpace(k), v)
		case "unset":
			if len(args) != 3 {
				log.Fatal("uso: env global unset CLAVE")
			}
			if err := cfgfile.UnsetGlobalEnv(db, args[2]); err != nil {
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
		k = strings.TrimSpace(k)
		if _, known := settings.Get(k); !known {
			log.Fatalf("settings: clave desconocida %q (usa: settings list)", k)
		}
		if err := settings.Validate(k, v); err != nil {
			log.Fatalf("settings: %v", err)
		}
		if err := db.SetSetting(k, v); err != nil {
			log.Fatalf("settings: %v", err)
		}
		fmt.Printf("%s=%s\n", k, v)
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
	includeSecrets := false
	pos := []string{}
	for _, a := range args {
		switch a {
		case "--include-secrets":
			includeSecrets = true
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) > 1 {
		log.Fatal("uso: config export [salida] [--include-secrets]")
	}
	if len(pos) == 1 {
		out = pos[0]
	}
	cfg, db := merged(cfgPath)
	defer db.Close()
	settings := map[string]string{}
	for _, k := range store.KnownSettings() {
		if v := db.GetSetting(k); v != "" {
			settings[k] = v
		}
	}
	export := config.ExportJSON
	if includeSecrets {
		export = config.ExportJSONWithSecrets
	}
	data, err := export(cfg, settings)
	if err != nil {
		log.Fatalf("export: %v", err)
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		log.Fatalf("export: %v", err)
	}
	_ = os.Chmod(out, 0o600)
	if includeSecrets {
		fmt.Printf("exportado a %s CON secretos (0600). No lo subas a git ni lo compartas.\n", out)
		return
	}
	fmt.Printf("exportado a %s (redactado; usa --include-secrets para restaurar)\n", out)
}

func cmdImport(args []string, cfgPath string) {
	if len(args) != 1 {
		log.Fatal("uso: config import <archivo>")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	prev, db := merged(cfgPath)
	defer db.Close()
	cfg, settings, err := config.ImportJSONMerged(data, prev)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	if err := cfgfile.ReplaceAll(db, cfg, settings); err != nil {
		log.Fatalf("import: %v", err)
	}
	fmt.Println("configuración importada (reinicio requerido para apps)")
}

func cmdAudit(args []string, cfgPath string) {
	rest, asJSON := wantJSON(args)
	limit := 100
	if len(rest) > 1 {
		log.Fatal("uso: audit [n]")
	}
	if len(rest) == 1 {
		n, err := strconv.Atoi(rest[0])
		if err != nil || n <= 0 {
			log.Fatal("audit: n inválido")
		}
		limit = n
	}
	_, db := merged(cfgPath)
	defer db.Close()
	rows, err := db.ListAudit(limit)
	if err != nil {
		log.Fatalf("audit: %v", err)
	}
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(rows)
		return
	}
	for _, a := range rows {
		fmt.Printf("%s\t%s\t%s\t%s\n", a.Ts, a.Actor, a.Action, a.Target)
	}
}

func cmdAlerts(args []string, cfgPath string) {
	rest, asJSON := wantJSON(args)
	sub := "list"
	if len(rest) > 0 {
		sub = rest[0]
	}
	_, db := merged(cfgPath)
	defer db.Close()
	switch sub {
	case "list":
		rules, err := db.ListRules()
		if err != nil {
			log.Fatalf("alerts: %v", err)
		}
		if asJSON {
			json.NewEncoder(os.Stdout).Encode(rules)
			return
		}
		for _, r := range rules {
			state := "on"
			if !r.Enabled {
				state = "off"
			}
			fmt.Printf("#%d\t%s\t%s\t%.2f\t%dmin\t%s\n", r.ID, r.App, r.Kind, r.Threshold, r.WindowMin, state)
		}
	case "channels":
		list, err := db.ListChannels()
		if err != nil {
			log.Fatalf("alerts channels: %v", err)
		}
		if asJSON {
			json.NewEncoder(os.Stdout).Encode(list)
			return
		}
		for _, c := range list {
			fmt.Printf("#%d\t%s\t%s\n", c.ID, c.Name, c.URL)
		}
	default:
		log.Fatal("alerts: subcomando list|channels")
	}
}

func cmdCron(args []string, cfgPath string) {
	rest, asJSON := wantJSON(args)
	app := ""
	if len(rest) > 0 && rest[0] != "list" {
		app = rest[0]
	}
	cfg, db := merged(cfgPath)
	defer db.Close()
	type out struct {
		App      string          `json:"app"`
		Name     string          `json:"name"`
		Schedule string          `json:"schedule"`
		Type     string          `json:"type"`
		Disabled bool            `json:"disabled"`
		Last     []store.CronRun `json:"-"`
	}
	rows := []out{}
	for _, a := range cfg.Apps {
		if app != "" && a.Name != app {
			continue
		}
		for _, j := range a.Cron {
			rows = append(rows, out{App: a.Name, Name: j.Name, Schedule: j.Schedule, Type: j.Kind(), Disabled: j.Disabled})
		}
	}
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(rows)
		return
	}
	for _, r := range rows {
		fmt.Printf("%s/%s\t%s\t%s\n", r.App, r.Name, r.Schedule, r.Type)
	}
	_ = db
}

func cmdDomains(args []string, cfgPath string) {
	_, asJSON := wantJSON(args)
	cfg, db := merged(cfgPath)
	defer db.Close()
	zones := map[string][]string{}
	for _, r := range cfg.Routes {
		for _, h := range r.Hosts {
			zones[h] = append(zones[h], r.Target)
		}
	}
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(zones)
		return
	}
	keys := make([]string, 0, len(zones))
	for k := range zones {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, h := range keys {
		fmt.Printf("%-40s -> %s\n", h, strings.Join(zones[h], ","))
	}
}

func cmdBackup(args []string, cfgPath string) {
	if len(args) > 1 {
		log.Fatal("uso: backup [destino]")
	}
	dest := filepath.Join(logx.DirFor(cfgPath), "backups", "rkt-"+time.Now().UTC().Format("20060102-150405")+".db")
	if len(args) == 1 {
		dest = args[0]
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatalf("backup: %v", err)
	}
	defer db.Close()
	if err := db.Backup(dest); err != nil {
		log.Fatalf("backup: %v", err)
	}
	fmt.Printf("backup ok: %s\n", dest)
}

func cmdRestore(args []string, cfgPath string) {
	if len(args) != 1 {
		log.Fatal("uso: restore <archivo.db>")
	}
	if cfg, err := cfgfile.Load(cfgPath); err == nil && cfg.Healthz.Enabled {
		client := &http.Client{Timeout: 2 * time.Second}
		if resp, err := client.Get("http://" + cfg.Healthz.Listen + "/healthz"); err == nil {
			resp.Body.Close()
			log.Fatal("restore: detén el servicio antes de restaurar (sc.exe stop Rkt)")
		}
	}
	src := args[0]
	if err := store.CheckFile(src); err != nil {
		log.Fatalf("restore: %v", err)
	}
	dst := logx.DBPath(cfgPath)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		log.Fatalf("restore: %v", err)
	}
	// Copia atómica: temp en el mismo volumen y rename.
	tmp := dst + ".restore"
	in, err := os.Open(src)
	if err != nil {
		log.Fatalf("restore: %v", err)
	}
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		in.Close()
		log.Fatalf("restore: %v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		in.Close()
		out.Close()
		os.Remove(tmp)
		log.Fatalf("restore: %v", err)
	}
	in.Close()
	out.Close()
	_ = os.Remove(dst + "-wal")
	_ = os.Remove(dst + "-shm")
	if err := os.Rename(tmp, dst); err != nil {
		log.Fatalf("restore: %v", err)
	}
	fmt.Printf("restore ok: %s (arranca el servicio)\n", dst)
}

// cmdReset borra la DB y la recrea de cero preservando solo users.
// Sesiones y API tokens se revocan: re-login y genera nueva key MCP.
// Exige servicio detenido y deja backup completo antes de borrar.
func cmdReset(args []string, cfgPath string) {
	if len(args) != 0 {
		log.Fatal("uso: reset  (sin argumentos; preserva solo users)")
	}
	if cfg, err := cfgfile.Load(cfgPath); err == nil && cfg.Healthz.Enabled {
		client := &http.Client{Timeout: 2 * time.Second}
		if resp, err := client.Get("http://" + cfg.Healthz.Listen + "/healthz"); err == nil {
			resp.Body.Close()
			log.Fatal("reset: detén el servicio antes (sc.exe stop Rkt)")
		}
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatalf("reset: %v", err)
	}
	users, err := db.DumpUsers()
	if err != nil {
		db.Close()
		log.Fatalf("reset: %v", err)
	}
	if len(users) == 0 {
		db.Close()
		log.Fatal("reset: sin users, nada que preservar; abortado")
	}
	dest := filepath.Join(logx.DirFor(cfgPath), "backups", "rkt-pre-reset-"+time.Now().UTC().Format("20060102-150405")+".db")
	if err := db.Backup(dest); err != nil {
		db.Close()
		log.Fatalf("reset: %v", err)
	}
	db.Close()
	dst := logx.DBPath(cfgPath)
	_ = os.Remove(dst + "-wal")
	_ = os.Remove(dst + "-shm")
	if err := os.Remove(dst); err != nil {
		log.Fatalf("reset: %v", err)
	}
	fresh, err := store.Open(dst)
	if err != nil {
		log.Fatalf("reset: %v", err)
	}
	defer fresh.Close()
	if err := fresh.ImportUsers(users); err != nil {
		log.Fatalf("reset: %v (restaura %s con restore)", err, dest)
	}
	fmt.Printf("reset ok: %d users preservados, backup en %s (sesiones y API tokens revocados)\n", len(users), dest)
}

// cmdDoctor diagnostica sin mutar: config, DB, servicio, git y puertos.
func cmdDoctor(_ []string, cfgPath string) {
	fail := 0
	ok := func(label string, err error) {
		if err != nil {
			fail++
			fmt.Printf("✗ %s: %v\n", label, err)
			return
		}
		fmt.Printf("✓ %s\n", label)
	}
	cfg, err := cfgfile.Load(cfgPath)
	ok("config", err)
	if err == nil {
		ok("puertos", doctorPorts(cfg))
		client := &http.Client{Timeout: 2 * time.Second}
		var resp *http.Response
		var herr error
		if cfg.Healthz.Enabled {
			resp, herr = client.Get("http://" + cfg.Healthz.Listen + "/healthz")
		} else {
			herr = fmt.Errorf("healthz desactivado")
		}
		if herr == nil {
			var st struct {
				Status string `json:"status"`
			}
			_ = jsonDecode(resp, &st)
			resp.Body.Close()
			if st.Status == "" {
				st.Status = "desconocido"
			}
			fmt.Printf("✓ servicio (%s)\n", st.Status)
			if st.Status != "ok" {
				fail++
				fmt.Println("  ⚠ estado degradado; revisa logs y backends")
			}
		} else {
			fail++
			fmt.Printf("✗ servicio: %v\n", herr)
		}
	}
	if _, err := os.Stat(logx.DBPath(cfgPath)); err != nil {
		fmt.Println("! base de datos: aún no creada (arranca el servicio para sembrarla)")
	} else {
		ok("base de datos", store.CheckFile(logx.DBPath(cfgPath)))
	}
	if _, err := exec.LookPath("git"); err != nil {
		fail++
		fmt.Printf("✗ git: %v\n", err)
	} else {
		fmt.Println("✓ git")
	}
	if fail > 0 {
		log.Fatalf("doctor: %d problema(s)", fail)
	}
	fmt.Println("doctor: todo ok")
}

// doctorPorts valida que los listeners no estén ocupados por otro proceso
// cuando el servicio está parado; si escucha, se asume el propio rkt.
func doctorPorts(cfg *config.Config) error {
	for _, addr := range []string{cfg.ListenHTTP, cfg.ListenHTTPS} {
		if addr == "" {
			continue
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			// Ocupado puede ser el propio rkt: no es error si healthz vive.
			continue
		}
		ln.Close()
	}
	return nil
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
	resp, err := controlPost(cfgPath, "http://"+cfg.Healthz.Listen+"/rollback/"+args[0], deployTimeout)
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

// openBrowser abre URL (best effort); la URL va como argumento directo.
func openBrowser(link string) {
	_ = platform.OpenBrowser(link)
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
