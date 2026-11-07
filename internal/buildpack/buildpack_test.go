package buildpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectAstroEsEstatico(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"astro build"},"dependencies":{"astro":"^7"}}`)
	write(t, dir, "pnpm-lock.yaml", "")

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "astro" || p.PM != "pnpm" {
		t.Fatalf("plan = %+v, quiero astro con pnpm", p)
	}
	if !p.IsStatic() || p.StaticDir != "dist" {
		t.Fatalf("static = %q, quiero dist", p.StaticDir)
	}
	if len(p.Start) != 0 {
		t.Fatalf("astro estático no debe arrancar proceso: %v", p.Start)
	}
	if len(p.Build) != 1 || p.Build[0] != "pnpm run build" {
		t.Fatalf("build = %v", p.Build)
	}
}

func TestDetectNextArranca(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"next build","start":"next start"},"dependencies":{"next":"^15"}}`)

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "next" || p.IsStatic() {
		t.Fatalf("plan = %+v, quiero next con proceso", p)
	}
	if strings.Join(p.Start, " ") != "pnpm run start" {
		t.Fatalf("start = %v", p.Start)
	}
	if p.PM != "pnpm" || len(p.Install) != 1 || p.Install[0] != "pnpm install" {
		t.Fatalf("pnpm obligatorio: %+v", p)
	}
}

func TestDetectPnpmFrozenConLock(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"next build","start":"next start"},"dependencies":{"next":"^15"}}`)
	write(t, dir, "pnpm-lock.yaml", "")
	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Install) != 1 || p.Install[0] != "pnpm install --frozen-lockfile" {
		t.Fatalf("install = %v", p.Install)
	}
}

func TestDetectYarnGanaYnpmNunca(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"dependencies":{}}`)
	write(t, dir, "yarn.lock", "")
	write(t, dir, "package-lock.json", "{}")

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.PM != "yarn" || p.Install[0] != "yarn install" {
		t.Fatalf("yarn debe ganar: %+v", p)
	}

	dir2 := t.TempDir()
	write(t, dir2, "package.json", `{"dependencies":{}}`)
	write(t, dir2, "package-lock.json", "{}")
	p2, err := DetectDir(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if p2.PM != "pnpm" || p2.Install[0] != "pnpm install" {
		t.Fatalf("npm nunca gana: %+v", p2)
	}
}

func TestDetectBunYWorkspace(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"workspaces":["apps/*"],"engines":{"node":">=20"}}`)
	write(t, dir, "bun.lock", "")
	write(t, dir, ".nvmrc", "v22.1.0\n")

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.PM != "bun" || !p.Workspace || p.NodeVersion != ">=20" {
		t.Fatalf("plan = %+v", p)
	}
	if !contains(p.Evidence, "workspaces") || !contains(p.Evidence, "bun.lock") {
		t.Fatalf("evidence = %v", p.Evidence)
	}
}

func TestDetectPythonUVYPoetry(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "uv.lock", "")
	write(t, dir, "pyproject.toml", "")
	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "python" || p.Install[0] != "uv sync" {
		t.Fatalf("uv = %+v", p)
	}

	dir2 := t.TempDir()
	write(t, dir2, "poetry.lock", "")
	write(t, dir2, "main.py", "")
	p2, err := DetectDir(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Install[0] != "poetry install" || strings.Join(p2.Start, " ") != "python -u main.py" {
		t.Fatalf("poetry = %+v", p2)
	}
}

func TestDetectRust(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname = \"servicio\"\n")
	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "rust" || !strings.Contains(p.Start[0], "servicio") {
		t.Fatalf("rust = %+v", p)
	}
	if cargoName("[package]\nname = 'x'") != "x" {
		t.Fatal("cargoName")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestDetectNodeServer(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"dependencies":{}}`)
	write(t, dir, "server.js", "")

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "node" || strings.Join(p.Start, " ") != "node server.js" {
		t.Fatalf("plan = %+v, quiero node server.js", p)
	}
}

func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module x")
	write(t, dir, "main.go", "package main")

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "go" || len(p.Build) == 0 {
		t.Fatalf("plan = %+v, quiero go con build", p)
	}
}

func TestDetectHTMLSueltoEsEstatico(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<html></html>")

	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "static" || p.StaticDir != "." {
		t.Fatalf("plan = %+v, quiero static .", p)
	}
}

func TestDetectSinSeñalesFalla(t *testing.T) {
	if _, err := DetectDir(t.TempDir()); err == nil {
		t.Fatal("quiero error accionable sin señales de proyecto")
	}
}

func TestFromPackageJSONRemoto(t *testing.T) {
	p := FromPackageJSON(`{"scripts":{"build":"vite build"},"devDependencies":{"vite":"^5"}}`, false)
	if p.Kind != "vite" || p.StaticDir != "dist" {
		t.Fatalf("plan remoto = %+v, quiero vite/dist", p)
	}
}

func TestFromRemoteSinPackageJSON(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]bool
		kind  string
	}{
		{"go", map[string]bool{"go.mod": true}, "go"},
		{"html raíz", map[string]bool{"index.html": true}, "static"},
		{"html en public", map[string]bool{"public/index.html": true}, "static"},
		{"python", map[string]bool{"requirements.txt": true, "app.py": true}, "python"},
	}
	for _, c := range cases {
		p := FromRemote("", c.files)
		if p.Kind != c.kind {
			t.Fatalf("%s: plan = %+v, quiero %s", c.name, p, c.kind)
		}
	}
	if p := FromRemote("", map[string]bool{"README.md": true}); p.Kind != "" {
		t.Fatalf("sin señales debe quedar vacío: %+v", p)
	}
}

func TestDevHint(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"dev":"vite"},"devDependencies":{"vite":"^5"}}`)
	got := DevHint(dir, "3010")
	if !strings.Contains(got, "pnpm exec vite") || !strings.Contains(got, "3010") {
		t.Fatalf("DevHint = %q", got)
	}
	if DevHint(t.TempDir(), "3010") != "" {
		t.Fatal("sin señales no debe sugerir comando")
	}
}

func TestDetectSolidStart(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{"scripts":{"build":"vinxi build"},"dependencies":{"@solidjs/start":"^1"}}`)
	p, err := DetectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "solidstart" || !strings.Contains(strings.Join(p.Start, " "), ".output") {
		t.Fatalf("solidstart = %+v", p)
	}
}

func TestDetectJavaYStaticBuild(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "pom.xml", "<project/>")
	p, err := DetectDir(dir)
	if err != nil || p.Kind != "java" || !strings.Contains(p.Build[0], "mvn") {
		t.Fatalf("java = %+v %v", p, err)
	}

	dir2 := t.TempDir()
	write(t, dir2, "build/index.html", "<html></html>")
	p2, err := DetectDir(dir2)
	if err != nil || p2.Kind != "static" || p2.StaticDir != "build" {
		t.Fatalf("static build = %+v %v", p2, err)
	}
}
