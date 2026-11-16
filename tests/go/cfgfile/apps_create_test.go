package cfgfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/store"
)

// Fixture mínima de proyecto: crea los archivos indicados.
func mkNode(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Matriz de inferencia local (sin Red): cada stack produce comando, args,
// env PORT y nombre derivado. El deploy solo existe con repo.
func TestCreateAppInfiereStacks(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		nodeBin  string
		pyBin    string
		wantCmd  string
		wantArgs []string
	}{
		{
			name: "node",
			files: map[string]string{
				"package.json": `{"name":"web"}`,
				"server.js":    "",
			},
			wantArgs: []string{"server.js"},
		},
		{
			name: "python",
			files: map[string]string{
				"requirements.txt": "flask\n",
				"app.py":           "",
			},
			wantArgs: []string{"-u", "app.py"},
		},
		{
			name:     "go",
			files:    map[string]string{"go.mod": "module example.com/x\n\ngo 1.21\n"},
			wantArgs: []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newTestDB(t)
			nodeBin := filepath.Join(t.TempDir(), "node-test", "node")
			if err := db.SetSetting(store.KeyNodeBin, nodeBin); err != nil {
				t.Fatal(err)
			}
			pyBin := filepath.Join(t.TempDir(), "py-test", "python")
			if err := db.SetSetting(store.KeyPythonBin, pyBin); err != nil {
				t.Fatal(err)
			}
			dir := mkNode(t, c.files)
			app, doms, portS, err := CreateApp(db, nil, "", "svc-"+c.name, "", 0, "", dir, false, "")
			if err != nil {
				t.Fatalf("CreateApp %s: %v", c.name, err)
			}
			wantCmd := c.wantCmd
			switch c.name {
			case "node":
				wantCmd = nodeBin
			case "python":
				wantCmd = pyBin
			}
			if wantCmd != "" && app.Command != wantCmd {
				t.Fatalf("command = %q, quiere %q", app.Command, wantCmd)
			}
			if app.Command == "" {
				t.Fatal("command vacío")
			}
			if len(app.Args) != len(c.wantArgs) {
				t.Fatalf("args = %v, quiere %v", app.Args, c.wantArgs)
			}
			for i := range c.wantArgs {
				if app.Args[i] != c.wantArgs[i] {
					t.Fatalf("args = %v, quiere %v", app.Args, c.wantArgs)
				}
			}
			if app.Env["PORT"] == "" || app.Env["PORT"] != portS {
				t.Fatalf("Env PORT = %q, portS = %q", app.Env["PORT"], portS)
			}
			if app.Deploy != nil {
				t.Fatal("directorio local no debe crear deploy")
			}
			if !filepath.IsAbs(app.Dir) {
				t.Fatalf("dir no absoluto: %q", app.Dir)
			}
			if len(doms) != 1 || !strings.HasSuffix(doms[0], ".localhost") {
				t.Fatalf("dominio local = %v", doms)
			}
		})
	}
}

// Estático puro (solo index.html): sin proceso, servido por el binario.
func TestCreateAppEstaticoPuro(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"index.html": "<h1>x</h1>"})
	app, _, _, err := CreateApp(db, nil, "", "sitio", "", 0, "", dir, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Static == "" {
		t.Fatal("Static vacío: index.html debe servirse como estático")
	}
	if len(app.Args) != 2 || app.Args[0] != "__static" {
		t.Fatalf("args = %v, quiere [__static dir]", app.Args)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if app.Command != self {
		t.Fatalf("command = %q, quiere binario propio %q", app.Command, self)
	}
	if app.Deploy != nil {
		t.Fatal("estático local no debe crear deploy")
	}
}

func TestCreateAppPuertoExplicitoManda(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"package.json": `{}`, "server.js": ""})
	app, _, portS, err := CreateApp(db, nil, "", "svc", "", 4567, "", dir, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if portS != "4567" || app.Env["PORT"] != "4567" {
		t.Fatalf("puerto explícito no respetado: %q %+v", portS, app.Env)
	}
}

func TestCreateAppNombreYDominioExplicitos(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"index.html": "<h1>x</h1>"})
	app, doms, _, err := CreateApp(db, nil, "", "MiApp", "a.test", 0, "", dir, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "MiApp" {
		t.Fatalf("nombre = %q, quiere MiApp", app.Name)
	}
	if len(doms) != 1 || doms[0] != "a.test" {
		t.Fatalf("domains = %v", doms)
	}
}

func TestCreateAppComandoExplicitoManda(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"package.json": `{"name":"x"}`, "server.js": ""})
	app, _, _, err := CreateApp(db, nil, "", "svc", "", 0, `C:\tools\node.exe`, dir, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Command != `C:\tools\node.exe` {
		t.Fatalf("command = %q", app.Command)
	}
	if len(app.Args) != 0 {
		t.Fatalf("args con comando explícito = %v", app.Args)
	}
}

func TestCreateAppAutoSinRepoNoDespliega(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"package.json": `{"name":"x"}`, "server.js": ""})
	app, _, _, err := CreateAndAdd(db, nil, "", "", "auto1", nil, 4601, `C:\x\node.exe`, dir, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Deploy != nil {
		t.Fatal("auto sin repo no debe crear bloque deploy")
	}
	fleet, err := db.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range fleet.Apps {
		if a.Name == "auto1" {
			found = true
		}
	}
	if !found {
		t.Fatal("app no persistida")
	}
}

func TestCreateAppDuplicadoFalla(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"index.html": "<h1>x</h1>"})
	if _, _, _, err := CreateAndAdd(db, nil, "", "", "dup", []string{"dup.test"}, 4602, `C:\x\rkt.exe`, dir, false, ""); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := CreateAndAdd(db, nil, "", "", "dup", []string{"otra.test"}, 4603, `C:\x\rkt.exe`, dir, false, "")
	if err == nil || !strings.Contains(err.Error(), "ya existe") {
		t.Fatalf("duplicado debió fallar con ya existe: %v", err)
	}
}

func TestCreateAppSinNombreFalla(t *testing.T) {
	db := newTestDB(t)
	_, _, _, err := CreateApp(db, nil, "", "", "", 0, "", "", false, "")
	if err == nil || !strings.Contains(err.Error(), "sin nombre") {
		t.Fatalf("err = %v, quiere 'sin nombre'", err)
	}
}

func TestCreateAppAppsBaseSinDir(t *testing.T) {
	db := newTestDB(t)
	base := t.TempDir()
	app, _, _, err := CreateApp(db, nil, "", "nueva", "", 0, `C:\x\node.exe`, "", false, base)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "nueva")
	if app.Dir != want {
		t.Fatalf("dir = %q, quiere %q", app.Dir, want)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Fatalf("dir no creado: %v", err)
	}
}

// Externas: sin proceso, sin deploy, sin build.
func TestCreateExternalRequiereDir(t *testing.T) {
	db := newTestDB(t)
	_, _, _, err := CreateExternalApp(db, "x", "x.test", filepath.Join(t.TempDir(), "no-existe"), 0)
	if err == nil || !strings.Contains(err.Error(), "directorio no existe") {
		t.Fatalf("err = %v, quiere directorio inexistente", err)
	}
}

func TestCreateExternalPlanoEstaticoIgnorado(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"index.html": "<h1>x</h1>"})
	app, _, portS, err := CreateExternalApp(db, "ext", "ext.test", dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if app.Command != "" || app.Deploy != nil || len(app.Args) != 0 || app.Static != "" {
		t.Fatalf("externa no debe inferir plan: %+v", app)
	}
	if portS == "" {
		t.Fatal("puerto vacío: la externa necesita puerto")
	}
	if app.Env["PORT"] != portS {
		t.Fatalf("Env PORT = %q, quiere %q", app.Env["PORT"], portS)
	}
}

func TestCreateExternalPuertoExplicito(t *testing.T) {
	db := newTestDB(t)
	dir := mkNode(t, map[string]string{"index.html": "<h1>x</h1>"})
	app, _, portS, err := CreateExternalApp(db, "ext", "ext.test", dir, 3210)
	if err != nil {
		t.Fatal(err)
	}
	if portS != "3210" || app.Env["PORT"] != "3210" {
		t.Fatalf("puerto = %q %+v", portS, app.Env)
	}
}
