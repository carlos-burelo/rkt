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
	if strings.Join(p.Start, " ") != "npm run start" {
		t.Fatalf("start = %v", p.Start)
	}
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
