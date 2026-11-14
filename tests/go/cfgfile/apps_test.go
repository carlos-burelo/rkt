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
func TestCreateExternalApp(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app, doms, port, err := CreateExternalApp(db, "dev", "dev.localhost", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if app.Runtime != "external" || app.Command != "" || app.Deploy != nil {
		t.Fatalf("app externa mal: %+v", app)
	}
	if port == "" || len(doms) != 1 || doms[0] != "dev.localhost" {
		t.Fatalf("port/domains = %q %v", port, doms)
	}
	if err := AddApp(db, app, doms, port); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := CreateExternalApp(db, "otra", "otra.localhost", dir, 0); err != nil {
		t.Fatal(err)
	}
	// mismo puerto explícito debe fallar al agregar
	app2, doms2, _, _ := CreateExternalApp(db, "dup", "dup.localhost", dir, 3000)
	app2.Env["PORT"] = port
	if _, _, _, err := CreateExternalApp(db, "dup2", "dup2.localhost", dir, 3000); err != nil {
		t.Fatal(err)
	}
	if err := AddApp(db, app2, doms2, port); err == nil {
		t.Fatal("puerto duplicado debe fallar")
	}
}

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

	app, _, _, err := CreateApp(db, nil, "", "", "", 0, "", dir, false, "")
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

	app, _, _, err := CreateApp(db, nil, "", "coatl-in", "", 0, "", dir, false, "")
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
	_, _, _, err := CreateApp(db, nil, "", "x", "", 0, "", filepath.Join(t.TempDir(), "no-existe"), false, "")
	if err == nil || !strings.Contains(err.Error(), "directorio no existe") {
		t.Fatalf("err = %v, quiero error de directorio inexistente", err)
	}
}

func TestCreateAppDominioPorDefectoSegunModo(t *testing.T) {
	db := newTestDB(t)
	site := t.TempDir()
	if err := os.WriteFile(filepath.Join(site, "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(store.KeyBaseDomain, "ejemplo.com"); err != nil {
		t.Fatal(err)
	}
	// Sin IP pública manda el dominio local, aunque haya base_domain.
	if _, hosts, _, err := CreateApp(db, nil, "", "sitio", "", 0, "", site, false, ""); err != nil {
		t.Fatal(err)
	} else if len(hosts) != 1 || hosts[0] != "sitio.localhost" {
		t.Fatalf("local = %v", hosts)
	}
	// Con IP pública usa el base_domain.
	if err := db.SetSetting(store.KeyPublicIP, "203.0.113.7"); err != nil {
		t.Fatal(err)
	}
	if _, hosts, _, err := CreateApp(db, nil, "", "sitio", "", 0, "", site, false, ""); err != nil {
		t.Fatal(err)
	} else if len(hosts) != 1 || hosts[0] != "sitio.ejemplo.com" {
		t.Fatalf("público = %v", hosts)
	}
	// Servidor público sin base ni dominio explícito: no inventa localhost.
	if err := db.SetSetting(store.KeyBaseDomain, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := CreateApp(db, nil, "", "sitio", "", 0, "", site, false, ""); err == nil ||
		!strings.Contains(err.Error(), "IP pública") {
		t.Fatalf("err = %v, quiero solicitud de dominio", err)
	}
}

func TestCreateAppSinPlanFalla(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir() // sin package.json ni requirements
	_, _, _, err := CreateApp(db, nil, "", "x", "", 0, "", dir, false, "")
	if err == nil || !strings.Contains(err.Error(), "sin plan de build") {
		t.Fatalf("err = %v, quiero error de plan no detectado", err)
	}
}

// Sin dir, la app cae bajo appsBase (home/apps); con "" cae al default
// de plataforma. Es el anclaje al home único.
func TestCreateAppRespetaAppsBase(t *testing.T) {
	db := newTestDB(t)
	home := t.TempDir()
	app, _, _, err := CreateApp(db, nil, "", "hogar", "", 0, "node", "", false, filepath.Join(home, "apps"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "apps", "hogar")
	if app.Dir != want {
		t.Fatalf("dir = %q, quiere %q", app.Dir, want)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Fatalf("dir no creado: %v", want)
	}
}
