// Package buildpack: detección y plan de build propio (estilo buildpacks,
// sin Docker ni nix). Un solo hogar para decidir cómo instalar, compilar
// y arrancar un proyecto; CLI, alta de apps y deploy lo consumen.
package buildpack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Plan plan canónico para un proyecto.
type Plan struct {
	// Kind identidad legible: astro, next, vite, sveltekit, nuxt, remix,
	// node, go, python, static.
	Kind string
	// PM gestor de paquetes: siempre pnpm (nunca npm: lento e inseguro).
	PM string
	// Install steps de instalación (shell).
	Install []string
	// Build steps de compilación (shell).
	Build []string
	// Start argv de arranque (sin shell). Vacío si es estático.
	Start []string
	// StaticDir directorio a servir, relativo al proyecto. "" si no es estático.
	StaticDir string
	// Evidence manifiestos y lockfiles que justifican el plan.
	Evidence []string
	// NodeVersion versión declarada (.nvmrc o engines.node); "" = default.
	NodeVersion string
	// Workspace indica monorepo/workspaces detectado.
	Workspace bool
}

// IsStatic indica que el resultado se sirve como archivos, sin proceso.
func (p Plan) IsStatic() bool { return p.StaticDir != "" }

// Static resuelve el directorio estático absoluto dentro de root.
func (p Plan) Static(root string) string {
	if p.StaticDir == "" {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(p.StaticDir))
}

// DetectDir inspecciona un proyecto en disco y devuelve su plan.
func DetectDir(dir string) (Plan, error) {
	exists := func(name string) bool {
		st, err := os.Stat(filepath.Join(dir, name))
		return err == nil && !st.IsDir()
	}
	pkg, hasPkg := "", false
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		pkg, hasPkg = string(b), true
	}
	plan := detect(pkg, hasPkg,
		func(name string) bool { return exists(name) },
		func() []string {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return nil
			}
			out := make([]string, 0, len(entries))
			for _, e := range entries {
				out = append(out, e.Name())
			}
			return out
		})
	if plan.Kind == "" {
		return Plan{}, fmt.Errorf("no reconocí el proyecto en %s: falta package.json, go.mod, Cargo.toml, requirements.txt, pyproject.toml, composer.json, Gemfile, deno.json, *.csproj, mkdocs.yml, hugo.toml o index.html", dir)
	}
	if plan.Kind == "node" && plan.NodeVersion == "" {
		if b, err := os.ReadFile(filepath.Join(dir, ".nvmrc")); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				plan.NodeVersion = strings.TrimPrefix(v, "v")
				plan.Evidence = append(plan.Evidence, ".nvmrc")
			}
		}
	}
	if plan.Kind == "rust" {
		if b, err := os.ReadFile(filepath.Join(dir, "Cargo.toml")); err == nil {
			name := cargoName(string(b))
			if name != "" {
				bin := name
				if runtime.GOOS == "windows" && filepath.Ext(bin) == "" {
					bin += ".exe"
				}
				plan.Start = []string{filepath.Join("target", "release", bin)}
			}
		}
	}
	return plan, nil
}

// cargoName extrae package.name de un Cargo.toml mínimo.
func cargoName(data string) string {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "name") {
			continue
		}
		if i := strings.IndexByte(line, '='); i >= 0 {
			return strings.Trim(strings.TrimSpace(line[i+1:]), `"'`)
		}
	}
	return ""
}

// FromPackageJSON clasifica un proyecto remoto del que solo conocemos
// package.json (y si tiene requirements.txt). Sin árbol no adivina más.
func FromPackageJSON(pkg string, hasRequirements bool) Plan {
	files := map[string]bool{}
	if hasRequirements {
		files["requirements.txt"] = true
	}
	return FromRemote(pkg, files)
}

// FromRemote clasifica con el package.json (vacío si no hay) y los nombres
// de archivos conocidos en la raíz del repo (go.mod, requirements.txt,
// index.html, public/index.html). La lista la trae el caller (API GitHub).
func FromRemote(packageJSON string, files map[string]bool) Plan {
	return detect(packageJSON, strings.TrimSpace(packageJSON) != "", func(name string) bool {
		return files[name]
	}, func() []string {
		out := make([]string, 0, len(files))
		for n := range files {
			out = append(out, n)
		}
		return out
	})
}

type pkgJSON struct {
	Scripts      map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
	Engines      map[string]string `json:"engines"`
	Workspaces   json.RawMessage   `json:"workspaces"`
}

func (p pkgJSON) dep(name string) bool {
	if _, ok := p.Dependencies[name]; ok {
		return true
	}
	_, ok := p.DevDeps[name]
	return ok
}

// packageManager detecta lockfile; npm nunca gana (se usa pnpm).
func packageManager(file func(string) bool) string {
	switch {
	case file("bun.lockb") || file("bun.lock"):
		return "bun"
	case file("yarn.lock"):
		return "yarn"
	default:
		return "pnpm"
	}
}

func runCmd(pm, script string) []string {
	return []string{pm, "run", script}
}

func execCmd(pm, bin string, args ...string) []string {
	switch pm {
	case "yarn":
		return append([]string{"yarn", bin}, args...)
	case "bun":
		return append([]string{"bunx", bin}, args...)
	default:
		return append([]string{"pnpm", "exec", bin}, args...)
	}
}

// detect decide plan con los datos disponibles. file = existencia puntual;
// list = nombres del árbol (para patrones como *.csproj).
func detect(packageJSON string, hasPkg bool, file func(string) bool, list func() []string) Plan {
	// Binarios sin ecosistema de paquetes.
	if file("go.mod") {
		name := "app"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return Plan{
			Kind:  "go",
			Build: []string{"go build -trimpath -o " + name + " ."},
			// Relativo al slot con separador: se resuelve contra el dir
			// del canario/instancia (los slots .a/.b cambian por corte;
			// absoluto al slot viejo quedaría apuntando al anterior).
			Start:    []string{"." + string(filepath.Separator) + name},
			Evidence: []string{"go.mod"},
		}
	}
	if file("Cargo.toml") {
		return Plan{
			Kind:     "rust",
			Build:    []string{"cargo build --release"},
			Start:    []string{filepath.Join("target", "release", "app")},
			Evidence: []string{"Cargo.toml"},
		}
	}
	if file("pom.xml") {
		return Plan{
			Kind:     "java",
			Build:    []string{"mvn -q -DskipTests package"},
			Start:    []string{"mvn", "-q", "spring-boot:run"},
			Evidence: []string{"pom.xml"},
		}
	}
	if file("build.gradle") || file("build.gradle.kts") {
		wrapper := "./gradlew"
		if runtime.GOOS == "windows" {
			wrapper = "gradlew.bat"
		}
		return Plan{
			Kind:     "java",
			Build:    []string{wrapper + " build -x test"},
			Start:    []string{wrapper, "bootRun"},
			Evidence: []string{"build.gradle"},
		}
	}
	// .NET: el nombre del proyecto varía, se detecta por extensión y se
	// fija el ensamblado a "app" para un arranque determinista.
	if proj, ok := firstExt(list(), ".csproj", ".fsproj"); ok {
		return Plan{
			Kind:     "dotnet",
			Build:    []string{"dotnet publish -c Release -o ./publish -p:AssemblyName=app"},
			Start:    []string{"dotnet", "./publish/app.dll", "--urls", "http://127.0.0.1:$PORT"},
			Evidence: []string{proj},
		}
	}
	// Parseadores de package.json para frontSteps (Laravel/Shakapacker).
	var pjEarly pkgJSON
	_ = json.Unmarshal([]byte(packageJSON), &pjEarly)
	pmEarly := packageManager(file)
	hasEarly := func(script string) bool { _, ok := pjEarly.Scripts[script]; return ok }
	// frontSteps antepone install+build del frontend cuando el repo trae
	// package.json con script build (Laravel, Shakapacker).
	frontSteps := func() (install, build []string) {
		if hasPkg && hasEarly("build") {
			ins := pmEarly + " install"
			if pmEarly == "pnpm" && file("pnpm-lock.yaml") {
				ins = pmEarly + " install --frozen-lockfile"
			}
			install = []string{ins}
			build = []string{pmEarly + " run build"}
		}
		return install, build
	}
	if file("composer.json") {
		plan := Plan{Kind: "php", Evidence: []string{"composer.json"}}
		fi, fb := frontSteps()
		plan.Install = append([]string{"composer install --no-dev --optimize-autoloader"}, fi...)
		plan.Build = fb
		switch {
		case file("artisan"):
			plan.Start = []string{"php", "artisan", "serve", "--host", "127.0.0.1", "--port", "$PORT"}
			plan.Evidence = append(plan.Evidence, "artisan")
		case file("public/index.php"):
			plan.Start = []string{"php", "-S", "127.0.0.1:$PORT", "-t", "public"}
		case file("index.php"):
			plan.Start = []string{"php", "-S", "127.0.0.1:$PORT"}
		}
		if len(plan.Start) == 0 && len(fb) == 0 {
			return Plan{}
		}
		return plan
	}
	if file("Gemfile") {
		// Jekyll es estático (se sirve, no se ejecuta).
		if file("_config.yml") {
			return Plan{
				Kind: "static", Install: []string{"bundle install"},
				Build:     []string{"bundle exec jekyll build"},
				StaticDir: "_site", Evidence: []string{"_config.yml", "Gemfile"},
			}
		}
		plan := Plan{Kind: "ruby", Evidence: []string{"Gemfile"}}
		fi, fb := frontSteps()
		plan.Install = append([]string{"bundle install"}, fi...)
		plan.Build = fb
		if file("config.ru") {
			plan.Start = []string{"bundle", "exec", "puma", "-b", "tcp://127.0.0.1:$PORT", "-e", "production"}
			plan.Evidence = append(plan.Evidence, "config.ru")
		}
		if len(plan.Start) == 0 && len(fb) == 0 {
			return Plan{}
		}
		return plan
	}
	if file("deno.json") || file("deno.jsonc") {
		if file("fresh.config.ts") {
			// Fresh aún sin plan automático: el genérico pide comando manual.
			return Plan{}
		}
		entry := ""
		for _, e := range []string{"main.ts", "server.ts", "mod.ts", "main.js", "server.js"} {
			if file(e) {
				entry = e
				break
			}
		}
		if entry == "" {
			return Plan{}
		}
		ev := "deno.json"
		if file("deno.jsonc") {
			ev = "deno.jsonc"
		}
		return Plan{
			Kind:     "deno",
			Start:    []string{"deno", "serve", "--host", "127.0.0.1", "--port", "$PORT", entry},
			Evidence: []string{ev, entry},
		}
	}
	for _, f := range []string{"hugo.toml", "hugo.yaml", "hugo.json"} {
		if file(f) {
			return Plan{
				Kind: "static", Build: []string{"hugo --minify"},
				StaticDir: "public", Evidence: []string{f},
			}
		}
	}
	for _, f := range []string{"mkdocs.yml", "mkdocs.yaml"} {
		if file(f) {
			ins := []string{"pip install mkdocs"}
			if file("requirements.txt") {
				ins = []string{"pip install -r requirements.txt"}
			}
			return Plan{
				Kind: "static", Install: ins, Build: []string{"mkdocs build"},
				StaticDir: "site", Evidence: []string{f},
			}
		}
	}
	if file("uv.lock") || file("poetry.lock") || file("pyproject.toml") || file("requirements.txt") {
		entry := "app.py"
		if !file("app.py") {
			entry = "main.py"
		}
		install := []string{"pip install -r requirements.txt"}
		evidence := []string{"requirements.txt"}
		switch {
		case file("uv.lock"):
			install, evidence = []string{"uv sync"}, []string{"uv.lock"}
		case file("poetry.lock"):
			install, evidence = []string{"poetry install"}, []string{"poetry.lock"}
		case file("pyproject.toml"):
			install, evidence = []string{"pip install ."}, []string{"pyproject.toml"}
		}
		return Plan{
			Kind:     "python",
			Install:  install,
			Start:    []string{"python", "-u", entry},
			Evidence: evidence,
		}
	}

	if !hasPkg {
		if dir := pickStatic(file); dir != "" {
			return Plan{Kind: "static", StaticDir: dir, Evidence: []string{"index.html"}}
		}
		return Plan{}
	}

	var pj pkgJSON
	_ = json.Unmarshal([]byte(packageJSON), &pj)
	pm := packageManager(file)
	install := []string{pm + " install"}
	has := func(script string) bool { _, ok := pj.Scripts[script]; return ok }

	if pm == "pnpm" && file("pnpm-lock.yaml") {
		install = []string{pm + " install --frozen-lockfile"}
	}
	plan := Plan{Kind: "node", PM: pm, Install: install, Evidence: []string{"package.json"}}
	switch {
	case file("pnpm-lock.yaml"):
		plan.Evidence = append(plan.Evidence, "pnpm-lock.yaml")
	case file("yarn.lock"):
		plan.Evidence = append(plan.Evidence, "yarn.lock")
	case file("bun.lockb") || file("bun.lock"):
		plan.Evidence = append(plan.Evidence, "bun.lock")
	case file("package-lock.json"):
		plan.Evidence = append(plan.Evidence, "package-lock.json→pnpm")
	}
	if len(pj.Workspaces) > 0 || file("pnpm-workspace.yaml") {
		plan.Workspace = true
		plan.Evidence = append(plan.Evidence, "workspaces")
	}
	if v := strings.TrimSpace(pj.Engines["node"]); v != "" {
		plan.NodeVersion = v
	}

	switch {
	case pj.dep("next"):
		plan.Kind = "next"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		// El puerto lo manda rkt (PORT). `pnpm run start` puede fijar
		// `-p 3001` y desincronizar la app del proxy; Next start directo
		// respeta PORT y escucha solo loopback.
		plan.Start = execCmd(pm, "next", "start", "-H", "127.0.0.1")
	case pj.dep("@sveltejs/kit"):
		plan.Kind = "sveltekit"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		plan.Start = []string{"node", "build"}
	case pj.dep("nuxt"):
		plan.Kind = "nuxt"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		plan.Start = []string{"node", filepath.Join(".output", "server", "index.mjs")}
	case pj.dep("@remix-run/dev") || pj.dep("@remix-run/node"):
		plan.Kind = "remix"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		plan.Start = runCmd(pm, "start")
	case pj.dep("@solidjs/start"):
		plan.Kind = "solidstart"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		plan.Start = []string{"node", filepath.Join(".output", "server", "index.mjs")}
	case pj.dep("astro"):
		plan.Kind = "astro"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		plan.StaticDir = "dist"
	case pj.dep("vite"):
		plan.Kind = "vite"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		plan.StaticDir = "dist"
	default:
		// Node plano: script start manda; si no, un entry conocido.
		switch {
		case has("start"):
			plan.Start = runCmd(pm, "start")
		case pj.dep("serve") && has("build"):
			plan.Build = []string{pm + " run build"}
			plan.Start = execCmd(pm, "serve", "dist")
		case file("server.js"):
			plan.Start = []string{"node", "server.js"}
		case file("server.mjs"):
			plan.Start = []string{"node", "server.mjs"}
		case file("index.js"):
			plan.Start = []string{"node", "index.js"}
		case file("index.html"):
			plan.Kind = "static"
			plan.StaticDir = "."
		default:
			if has("build") {
				plan.Build = []string{pm + " run build"}
			}
			plan.Kind = "static"
			plan.StaticDir = "dist"
			if dir := pickStatic(file); dir != "" {
				plan.StaticDir = dir
			}
		}
	}
	return plan
}

// firstExt primer archivo con una de las extensiones (insensible a caso).
func firstExt(names []string, exts ...string) (string, bool) {
	for _, n := range names {
		for _, e := range exts {
			if strings.EqualFold(filepath.Ext(n), e) {
				return n, true
			}
		}
	}
	return "", false
}

// pickStatic elige una salida estática ya presente o el default dist.
func pickStatic(file func(string) bool) string {
	for _, d := range []string{".", "public", "dist", "build", "out", ".output/public"} {
		if file(filepath.ToSlash(filepath.Join(d, "index.html"))) {
			return d
		}
	}
	return ""
}

// DevHint sugiere el comando para levantar el dev server local. Solo es
// texto para el humano: rkt NO lo ejecuta en apps externas.
func DevHint(dir, port string) string {
	if dir == "" {
		return ""
	}
	if port == "" {
		port = "3000"
	}
	plan, err := DetectDir(dir)
	if err != nil {
		return ""
	}
	switch plan.Kind {
	case "next":
		return strings.Join(execCmd(pmOf(plan), "next", "dev", "-p", port, "-H", "127.0.0.1"), " ")
	case "vite":
		return strings.Join(execCmd(pmOf(plan), "vite", "--host", "127.0.0.1", "--port", port, "--strictPort"), " ")
	case "astro":
		return strings.Join(execCmd(pmOf(plan), "astro", "dev", "--host", "127.0.0.1", "--port", port), " ")
	case "sveltekit":
		return strings.Join(execCmd(pmOf(plan), "vite", "dev", "--host", "127.0.0.1", "--port", port, "--strictPort"), " ")
	case "nuxt":
		return strings.Join(execCmd(pmOf(plan), "nuxt", "dev", "--host", "127.0.0.1", "--port", port), " ")
	case "go":
		return "go run ."
	case "python":
		return "python -u app.py"
	case "php":
		return "php -S 127.0.0.1:" + port
	case "ruby":
		return "bundle exec rails server -b 127.0.0.1 -p " + port
	case "dotnet":
		return "dotnet run"
	case "deno":
		if len(plan.Start) > 0 {
			out := append([]string{plan.Start[0]}, plan.Start[1:]...)
			for i, a := range out {
				out[i] = strings.ReplaceAll(a, "$PORT", port)
			}
			return strings.Join(out, " ")
		}
		return ""
	case "static":
		return "" // sin proceso: se sirve estático
	}
	if plan.Kind != "" {
		return strings.Join(runCmd(pmOf(plan), "dev"), " ")
	}
	return ""
}

func pmOf(p Plan) string {
	if p.PM != "" {
		return p.PM
	}
	return "pnpm"
}
