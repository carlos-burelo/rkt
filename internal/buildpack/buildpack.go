// Package buildpack: detección y plan de build propio (estilo buildpacks,
// sin Docker ni nix). Un solo hogar para decidir cómo instalar, compilar
// y arrancar un proyecto; CLI, alta de apps y deploy lo consumen.
package buildpack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		return Plan{}, fmt.Errorf("no reconocí el proyecto en %s: falta package.json, go.mod, requirements.txt o index.html", dir)
	}
	return plan, nil
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
}

func (p pkgJSON) dep(name string) bool {
	if _, ok := p.Dependencies[name]; ok {
		return true
	}
	_, ok := p.DevDeps[name]
	return ok
}

// detect decide plan con los datos disponibles. file = existencia puntual.
func detect(packageJSON string, hasPkg bool, file func(string) bool) Plan {
	// Binarios sin ecosistema de paquetes.
	if file("go.mod") {
		name := "app.exe"
		return Plan{
			Kind:  "go",
			Build: []string{"go build -o " + name + " ."},
			Start: []string{filepath.Join(".", name)},
		}
	}
	if file("requirements.txt") {
		entry := "app.py"
		if !file("app.py") {
			entry = "main.py"
		}
		return Plan{
			Kind:    "python",
			Install: []string{"pip install -r requirements.txt"},
			Start:   []string{"python", "-u", entry},
		}
	}

	if !hasPkg {
		if file("index.html") || file("public/index.html") {
			root := "."
			if !file("index.html") && file("public/index.html") {
				root = "public"
			}
			return Plan{Kind: "static", StaticDir: root}
		}
		return Plan{}
	}

	var pj pkgJSON
	_ = json.Unmarshal([]byte(packageJSON), &pj)
	pm := "pnpm"
	install := []string{pm + " install"}
	has := func(script string) bool { _, ok := pj.Scripts[script]; return ok }

	plan := Plan{Kind: "node", PM: pm, Install: install}

	switch {
	case pj.dep("next"):
		plan.Kind = "next"
		if has("build") {
			plan.Build = []string{pm + " run build"}
		}
		if has("start") {
			plan.Start = []string{pm, "run", "start"}
		} else {
			plan.Start = []string{"pnpm", "exec", "next", "start"}
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
		plan.Start = []string{pm, "run", "start"}
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
			plan.Start = []string{pm, "run", "start"}
		case pj.dep("serve") && has("build"):
			plan.Build = []string{pm + " run build"}
			plan.Start = []string{"pnpm", "exec", "serve", "dist"}
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
