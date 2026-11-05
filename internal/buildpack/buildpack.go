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
	plan := detect(pkg, hasPkg, func(name string) bool { return exists(name) })
	if plan.Kind == "" {
		return Plan{}, fmt.Errorf("no reconocí el proyecto en %s: falta package.json, go.mod, requirements.txt, pyproject.toml, Cargo.toml o index.html", dir)
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

// detect decide plan con los datos disponibles. file = existencia puntual.
func detect(packageJSON string, hasPkg bool, file func(string) bool) Plan {
	// Binarios sin ecosistema de paquetes.
	if file("go.mod") {
		name := "app"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return Plan{
			Kind:     "go",
			Build:    []string{"go build -trimpath -o " + name + " ."},
			Start:    []string{filepath.Join(".", name)},
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
		if file("index.html") || file("public/index.html") {
			root := "."
			if !file("index.html") && file("public/index.html") {
				root = "public"
			}
			return Plan{Kind: "static", StaticDir: root, Evidence: []string{"index.html"}}
		}
		return Plan{}
	}

	var pj pkgJSON
	_ = json.Unmarshal([]byte(packageJSON), &pj)
	pm := packageManager(file)
	install := []string{pm + " install"}
	has := func(script string) bool { _, ok := pj.Scripts[script]; return ok }

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
		if has("start") {
			plan.Start = runCmd(pm, "start")
		} else {
			plan.Start = execCmd(pm, "next", "start")
		}
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
		}
	}
	return plan
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
