package cfgfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// El alta desde "directorio local" debe inferir plan y comando igual
// que desde un repo; era el bug de creación (400 sin binario).
func TestCreateAppDirectorioLocalInfiere(t *testing.T) {
	db := newTestDB(t)
	if err := db.SetSetting(store.KeyNodeBin, `C:\Program Files\nodejs\node.exe`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"coatl"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	app, _, _, err := CreateApp(db, "", "", "", 0, "", dir, false)
	if err != nil {
		t.Fatalf("CreateApp local: %v", err)
	}
	if app.Command != `C:\Program Files\nodejs\node.exe` {
		t.Fatalf("command = %q, quiero el node_bin configurado", app.Command)
	}
	if len(app.Args) != 1 || app.Args[0] != "server.js" {
		t.Fatalf("args = %v, quiero server.js", app.Args)
	}
	if app.Name == "" {
		t.Fatal("nombre vacío: debía derivarse del directorio")
	}
	if !filepath.IsAbs(app.Dir) {
		t.Fatalf("dir = %q, quiero absoluto", app.Dir)
	}
	if app.Deploy != nil {
		t.Fatal("directorio local no debe crear deploy de repo")
	}
}

// Astro/Vite: plan estático servido por el binario, sin proceso Node.
func TestCreateAppAstroServidoEstatico(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"build":"astro build"},"dependencies":{"astro":"^7"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app, _, _, err := CreateApp(db, "", "coatl-in", "", 0, "", dir, false)
	if err != nil {
		t.Fatalf("CreateApp astro: %v", err)
	}
	if app.Static == "" {
		t.Fatal("Static vacío: astro debía servirse como estático")
	}
	if len(app.Args) != 2 || app.Args[0] != "__static" {
		t.Fatalf("args = %v, quiero __static <dir>", app.Args)
	}
	if app.Command == "" {
		t.Fatal("command vacío: debe apuntar al binario propio")
	}
}

func TestCreateAppDirectorioInexistente(t *testing.T) {
	db := newTestDB(t)
	_, _, _, err := CreateApp(db, "", "x", "", 0, "", filepath.Join(t.TempDir(), "no-existe"), false)
	if err == nil || !strings.Contains(err.Error(), "directorio no existe") {
		t.Fatalf("err = %v, quiero error de directorio inexistente", err)
	}
}

func TestCreateAppSinPlanFalla(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir() // sin package.json ni requirements
	_, _, _, err := CreateApp(db, "", "x", "", 0, "", dir, false)
	if err == nil || !strings.Contains(err.Error(), "sin plan de build") {
		t.Fatalf("err = %v, quiero error de plan no detectado", err)
	}
}
