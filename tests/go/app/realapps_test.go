package app

// Flujo de deploy con datos reales (sin mocks): los fixtures viven en
// tests/apps/ y cada test los copia a un temporal, los vuelve repo git y
// corre el pipeline de verdad (clone + build + canario + espera HTTP +
// corte + rollback) con supervisor real. Lo que se afirma es observable:
// el cuerpo HTTP servido, el puerto vivo/muerto y el error devuelto.

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/supervisor"
)

// fixtureRoot localiza tests/apps/: override por env (CI) o relativo al
// archivo del test (go test usa el dir del paquete, con o sin overlay).
func fixtureRoot(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("RKT_TEST_APPS"); env != "" {
		if st, err := os.Stat(env); err == nil && st.IsDir() {
			return env
		}
		t.Fatalf("RKT_TEST_APPS=%q no es dir", env)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		cand := filepath.Join(filepath.Dir(file), "..", "..", "tests", "apps")
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs
			}
			return cand
		}
	}
	for _, cand := range []string{
		filepath.Join("..", "..", "tests", "apps"),
		filepath.Join("tests", "apps"),
	} {
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			return cand
		}
	}
	t.Fatal("tests/apps no localizado")
	return ""
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dst, rel)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	}); err != nil {
		t.Fatalf("copiar fixture: %v", err)
	}
}

// HOME aislado por invocación: git no debe leer ni escribir el Temp real.
func gitEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
	)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// fixtureRepo copia tests/apps/<name> a un temporal y lo vuelve repo git.
// Devuelve el path del repo (origen del clone del deploy).
func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
	src := filepath.Join(fixtureRoot(t), name)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		t.Fatalf("fixture %s ausente", name)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	copyDir(t, src, repo)
	gitRun(t, repo, "init", "-b", "main", ".")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "v1", "--no-gpg-sign")
	return repo
}

func commitAll(t *testing.T, repo, msg string) {
	t.Helper()
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", msg, "--no-gpg-sign")
}

// realWorld igual que bbWorld pero con comando/args/build del fixture.
// extraEnv se suma al entorno (ej GOTOOLCHAIN=local para no tocar red).
func realWorld(t *testing.T, cmd string, args []string, repo string, build []string, extraEnv map[string]string) (*store.DB, *config.Live, *supervisor.Manager, *supCtl) {
	t.Helper()
	return realWorldAt(t, t.TempDir(), cmd, args, repo, build, extraEnv)
}

// realWorldAt igual pero sobre un base propio (para reabrirlo tras un
// "reinicio": mismo disco, proceso nuevo).
func realWorldAt(t *testing.T, base, cmd string, args []string, repo string, build []string, extraEnv map[string]string) (*store.DB, *config.Live, *supervisor.Manager, *supCtl) {
	t.Helper()
	// Mismo layout que prod: cfg en base/, DB en base/logs/rkt.db.
	cfgPath := filepath.Join(base, "rkt.yaml")
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	env := map[string]string{"PORT": "1"}
	for k, v := range extraEnv {
		env[k] = v
	}
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps, config.App{
			Name: "bb", Command: cmd, Args: args,
			Dir: filepath.Join(base, "bb.a"), Env: env,
			RestartDelay: "5s", MinUptime: "1s",
			Deploy: &config.Deploy{Repo: repo, Ref: "main", Build: build},
		})
		cp.Routes = append(cp.Routes, config.Route{
			Hosts: []string{"bb.test"}, Target: "http://127.0.0.1:1",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := cfgfile.MutateInfra(db, func(cp *config.Config) error {
		cp.RateLimit.Enabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cp.RateLimit.Enabled = false
	live := config.NewLive(cp)
	sup := supervisor.New(cp.Apps, nil, filepath.Join(base, "logs"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(sup.Stop)
	return db, live, sup, &supCtl{sup: sup, db: db, live: live}
}

// Sin comillas ni ruta absoluta: el shell resuelve por PATH igual que
// pythonOrSkip (evita el parsing raro de `cmd /C "ruta" args` en Windows).
func pyCompileStep(py string) string {
	base := strings.TrimSuffix(filepath.Base(py), ".exe")
	return base + " -m py_compile app.py"
}

func nodeOrSkip(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("node"); err == nil {
		if exec.Command(path, "--version").Run() == nil {
			return path
		}
	}
	t.Skip("sin Node ejecutable")
	return ""
}

func nodeCheckStep() string { return "node --check server.js" }

func pnpmOrSkip(t *testing.T) {
	t.Helper()
	if path, err := exec.LookPath("pnpm"); err != nil {
		t.Skip("sin pnpm en PATH")
	} else if exec.Command(path, "--version").Run() != nil {
		t.Skip("pnpm no ejecutable")
	}
}

// TestRealNodeServerCorteYRollback: lo mismo que py pero con Node stdlib
// (proceso único: `node server.js` directo, sin wrapper pnpm).
func TestRealNodeServerCorteYRollback(t *testing.T) {
	node := nodeOrSkip(t)
	repo := fixtureRepo(t, "demo-node-server")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, live, _, ctl := realWorld(t, node, []string{"server.js"}, repo, []string{nodeCheckStep()}, nil)

	app := bbAppOf(live)
	if _, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy v1: %v", err)
	}
	p1 := bbPortOf(ctl.live)
	bbWaitServe(t, p1, "v1", 30*time.Second)

	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "v2")
	app = bbAppOf(ctl.live)
	if _, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy v2: %v", err)
	}
	p2 := bbPortOf(ctl.live)
	if p2 == "" || p2 == p1 {
		t.Fatalf("corte sin puerto nuevo: p1=%q p2=%q", p1, p2)
	}
	bbWaitServe(t, p2, "v2", 30*time.Second)
	if body, err := bbGet(t, p1); err == nil {
		t.Fatalf("la v1 sigue viva en %s con %q", p1, body)
	}

	if err := deploy.Rollback(bbAppOf(ctl.live), ctl, log); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	p3 := bbPortOf(ctl.live)
	if p3 == "" || p3 == p2 {
		t.Fatalf("rollback sin puerto nuevo: p2=%q p3=%q", p2, p3)
	}
	bbWaitServe(t, p3, "v1", 30*time.Second)
	if body, err := bbGet(t, p2); err == nil {
		t.Fatalf("la v2 sigue viva en %s con %q", p2, body)
	}
}

// TestRealPyDeployCorteYRollback: v1 sirve, v2 corta sin dejar muertos y el
// rollback devuelve v1. Todo con repo git y procesos de verdad.
func TestRealPyDeployCorteYRollback(t *testing.T) {
	py := pythonOrSkip(t)
	repo := fixtureRepo(t, "demo-py")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, live, _, ctl := realWorld(t, py, []string{"-u", "app.py"}, repo, []string{pyCompileStep(py)}, nil)

	app := bbAppOf(live)
	if _, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy v1: %v", err)
	}
	p1 := bbPortOf(ctl.live)
	bbWaitServe(t, p1, "v1", 30*time.Second)

	// v2: cambio real en el repo (VERSION) + commit.
	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "v2")
	app = bbAppOf(ctl.live)
	if _, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy v2: %v", err)
	}
	p2 := bbPortOf(ctl.live)
	if p2 == "" || p2 == p1 {
		t.Fatalf("corte sin puerto nuevo: p1=%q p2=%q", p1, p2)
	}
	bbWaitServe(t, p2, "v2", 30*time.Second)
	if body, err := bbGet(t, p1); err == nil {
		t.Fatalf("la v1 sigue viva en %s con %q", p1, body)
	}

	// Rollback real: vuelve a servir v1 y mata la v2.
	if err := deploy.Rollback(bbAppOf(ctl.live), ctl, log); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	p3 := bbPortOf(ctl.live)
	if p3 == "" || p3 == p2 {
		t.Fatalf("rollback sin puerto nuevo: p2=%q p3=%q", p2, p3)
	}
	bbWaitServe(t, p3, "v1", 30*time.Second)
	if body, err := bbGet(t, p2); err == nil {
		t.Fatalf("la v2 sigue viva en %s con %q", p2, body)
	}
}

// goBin pasos y arranque del fixture demo-go: binario compilado (un solo
// proceso, supervisado limpio) con path relativo al slot. `go run` genera
// un nieto fuera del job del canario adoptado y cuelga el Stop.
func goBin() (build, cmd string) {
	if runtime.GOOS == "windows" {
		return "go build -trimpath -o app.exe .", `.\\app.exe`
	}
	return "go build -trimpath -o app .", "./app"
}

// TestRealGoCompilaYSirve: build real del binario y canario real.
// GOTOOLCHAIN=local: hermético, sin descargas de toolchain por red.
func TestRealGoCompilaYSirve(t *testing.T) {
	repo := fixtureRepo(t, "demo-go")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	build, cmd := goBin()
	_, live, _, ctl := realWorld(t, cmd, nil, repo, []string{build}, map[string]string{"GOTOOLCHAIN": "local"})
	if _, err := deploy.RunContext(context.Background(), bbAppOf(live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy go: %v", err)
	}
	bbWaitServe(t, bbPortOf(ctl.live), "v1", 90*time.Second)
}

// TestRealNextConRed: stack completo con red de verdad dentro del pipeline:
// pnpm install + next build en el slot inactivo, canario del server ya
// compilado y corte. El arranque es `node` directo sobre el bin de next
// (proceso único supervisado limpio; ver nota sobre launchers pnpm).
func TestRealNextConRed(t *testing.T) {
	node := nodeOrSkip(t)
	pnpmOrSkip(t)
	repo := fixtureRepo(t, "demo-next")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	nextBin := filepath.Join("node_modules", "next", "dist", "bin", "next")
	_, live, _, ctl := realWorld(t, node, []string{nextBin, "start", "-H", "127.0.0.1"},
		repo, []string{"pnpm install", "pnpm run build"}, nil)

	app := bbAppOf(live)
	if _, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy next: %v", err)
	}
	port := bbPortOf(ctl.live)
	if port == "" || port == "1" {
		t.Fatalf("corte sin puerto nuevo: %q", port)
	}
	deadline := time.Now().Add(240 * time.Second)
	for {
		body, err := bbGet(t, port)
		if err == nil && strings.Contains(body, "demo-next-v1") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("next no sirvió el marcador (body=%q err=%v)", body, err)
		}
		time.Sleep(2 * time.Second)
	}
	if body, err := bbGet(t, "1"); err == nil {
		t.Fatalf("placeholder 1 no debe servir: %q", body)
	}
}

// TestRealViteConRed: install+build con red y dos versiones servidas con el
// preview del dist compilado (proceso único vía node directo).
func TestRealViteConRed(t *testing.T) {
	node := nodeOrSkip(t)
	pnpmOrSkip(t)
	repo := fixtureRepo(t, "demo-react-vite")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	viteBin := filepath.Join("node_modules", "vite", "bin", "vite.js")
	start := []string{viteBin, "preview", "--host", "127.0.0.1", "--port", "$PORT", "--strictPort"}
	_, live, _, ctl := realWorld(t, node, start,
		repo, []string{"pnpm install", "pnpm run build"}, nil)

	serveMarker := func(port, want string, timeout time.Duration) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for {
			body, err := bbGet(t, port)
			if err == nil && strings.Contains(body, want) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("vite no sirvió %q (body=%q err=%v)", want, body, err)
			}
			time.Sleep(2 * time.Second)
		}
	}

	if _, err := deploy.RunContext(context.Background(), bbAppOf(live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy vite v1: %v", err)
	}
	p1 := bbPortOf(ctl.live)
	serveMarker(p1, "demo-vite-v1", 120*time.Second)

	if err := os.WriteFile(filepath.Join(repo, "index.html"), []byte(`<!doctype html>
<html lang="es">
<head><meta charset="utf-8"><title>demo-vite-v2</title></head>
<body><div id="root"></div><script type="module" src="/src/main.jsx"></script></body>
</html>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "v2")
	if _, err := deploy.RunContext(context.Background(), bbAppOf(ctl.live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy vite v2: %v", err)
	}
	p2 := bbPortOf(ctl.live)
	if p2 == "" || p2 == p1 {
		t.Fatalf("corte sin puerto nuevo: p1=%q p2=%q", p1, p2)
	}
	serveMarker(p2, "demo-vite-v2", 120*time.Second)
	if body, err := bbGet(t, p1); err == nil {
		t.Fatalf("la v1 sigue viva en %s con %q", p1, body)
	}
}

// TestRealMultiAppAislamiento: dos apps reales (py + node) desplegadas en
// la misma flota, servidas por un solo proxy: cada host ve su versión y
// no hay cross-talk. Host desconocido da 404.
func TestRealMultiAppAislamiento(t *testing.T) {
	py := pythonOrSkip(t)
	node := nodeOrSkip(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pyRepo := fixtureRepo(t, "demo-py")
	nodeRepo := fixtureRepo(t, "demo-node-server")
	for _, tc := range []struct{ repo, v string }{{pyRepo, "py-v1"}, {nodeRepo, "node-v1"}} {
		if err := os.WriteFile(filepath.Join(tc.repo, "VERSION"), []byte(tc.v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		commitAll(t, tc.repo, tc.v)
	}

	base := t.TempDir()
	db, err := store.Open(filepath.Join(base, "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps,
			config.App{Name: "py", Command: py, Args: []string{"-u", "app.py"},
				Dir: filepath.Join(base, "py.a"), Env: map[string]string{"PORT": "1"},
				RestartDelay: "5s", MinUptime: "1s",
				Deploy: &config.Deploy{Repo: pyRepo, Ref: "main", Build: []string{pyCompileStep(py)}},
			},
			config.App{Name: "node", Command: node, Args: []string{"server.js"},
				Dir: filepath.Join(base, "node.a"), Env: map[string]string{"PORT": "2"},
				RestartDelay: "5s", MinUptime: "1s",
				Deploy: &config.Deploy{Repo: nodeRepo, Ref: "main", Build: []string{nodeCheckStep()}},
			},
		)
		cp.Routes = append(cp.Routes,
			config.Route{Hosts: []string{"py.test"}, Target: "http://127.0.0.1:1"},
			config.Route{Hosts: []string{"node.test"}, Target: "http://127.0.0.1:2"},
		)
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := cfgfile.MutateInfra(db, func(cp *config.Config) error {
		cp.RateLimit.Enabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cp.RateLimit.Enabled = false
	live := config.NewLive(cp)
	sup := supervisor.New(cp.Apps, nil, filepath.Join(base, "logs"), log)
	defer sup.Stop()
	ctl := &supCtl{sup: sup, db: db, live: live}

	for _, name := range []string{"py", "node"} {
		var app config.App
		for _, a := range live.Get().Apps {
			if a.Name == name {
				app = a
			}
		}
		if _, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log); err != nil {
			t.Fatalf("deploy %s: %v", name, err)
		}
	}

	front := httptest.NewServer(proxy.NewHandler(ctl.live, log))
	defer front.Close()
	getHost := func(host string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", front.URL, nil)
		req.Host = host
		resp, err := bbClient.Do(req)
		if err != nil {
			t.Fatalf("host %s: %v", host, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := getHost("py.test"); code != 200 || body != "py-v1" {
		t.Fatalf("py.test = %d %q", code, body)
	}
	if code, body := getHost("node.test"); code != 200 || body != "node-v1" {
		t.Fatalf("node.test = %d %q", code, body)
	}
	if code, _ := getHost("nadie.test"); code != 404 {
		t.Fatalf("host desconocido = %d, quiere 404", code)
	}
}

// TestRealNodePnpmWrapperSinHuerfanos: arranque vía `pnpm run start` como en
// prod (launcher con nieto). Con job-desde-el-inicio el corte mata el árbol
// y no quedan huérfanos sirviendo el puerto viejo. Sin el fix, p1 seguiría
// abierto y el Stop se colgaría.
func TestRealNodePnpmWrapperSinHuerfanos(t *testing.T) {
	nodeOrSkip(t)
	pnpmOrSkip(t)
	repo := fixtureRepo(t, "demo-node-server")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd.exe"
		args = []string{"/c", "pnpm", "run", "start"}
	} else {
		cmd = "/bin/sh"
		args = []string{"-c", "pnpm run start"}
	}
	_, live, sup, ctl := realWorld(t, cmd, args, repo, []string{nodeCheckStep()}, nil)

	if _, err := deploy.RunContext(context.Background(), bbAppOf(live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy wrapper v1: %v", err)
	}
	p1 := bbPortOf(ctl.live)
	bbWaitServe(t, p1, "v1", 30*time.Second)

	if err := os.WriteFile(filepath.Join(repo, "VERSION"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "v2")
	if _, err := deploy.RunContext(context.Background(), bbAppOf(ctl.live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy wrapper v2: %v", err)
	}
	p2 := bbPortOf(ctl.live)
	if p2 == "" || p2 == p1 {
		t.Fatalf("corte sin puerto nuevo: p1=%q p2=%q", p1, p2)
	}
	bbWaitServe(t, p2, "v2", 30*time.Second)
	if body, err := bbGet(t, p1); err == nil {
		t.Fatalf("nieto huérfano sirviendo en %s: %q", p1, body)
	}

	// El Stop debe volver pronto (antes se colgaba con el pipe retenido).
	done := make(chan struct{})
	go func() { sup.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Stop colgado: nieto del wrapper fuera del job")
	}
}

// TestRestartRecuperaAppsSirviendo: muerte del servicio y rearranque sobre
// el mismo disco (LoadMerged + NewLive + supervisor nuevo, como Run()).
// La app vuelve a servir sin redesplegar; rutas e historial intactos.
func TestRestartRecuperaAppsSirviendo(t *testing.T) {
	py := pythonOrSkip(t)
	repo := fixtureRepo(t, "demo-py")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	base := t.TempDir()
	cfgPath := filepath.Join(base, "rkt.yaml")
	db, live, sup, ctl := realWorldAt(t, base, py, []string{"-u", "app.py"}, repo, []string{pyCompileStep(py)}, nil)
	if _, err := deploy.RunContext(context.Background(), bbAppOf(live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy v1: %v", err)
	}
	p1 := bbPortOf(ctl.live)
	bbWaitServe(t, p1, "v1", 30*time.Second)

	// Muerte del servicio: se pierde todo lo vivo, solo queda el disco.
	sup.Stop()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// "Reboot": proceso nuevo sobre el mismo disco, mismo camino que Run().
	cfg, db2, err := cfgfile.LoadMerged(cfgPath)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	defer db2.Close()
	live2 := config.NewLive(cfg)
	sup2 := supervisor.New(cfg.Apps, cfg.GlobalEnv, filepath.Join(base, "logs"), log)
	defer sup2.Stop()
	sup2.Start()
	bbWaitServe(t, p1, "v1", 30*time.Second)

	// La ruta sobrevivió: el proxy sirve la app por su host.
	front := httptest.NewServer(proxy.NewHandler(live2, log))
	defer front.Close()
	req, _ := http.NewRequest("GET", front.URL, nil)
	req.Host = "bb.test"
	resp, err := bbClient.Do(req)
	if err != nil {
		t.Fatalf("proxy tras reboot: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "v1" {
		t.Fatalf("proxy sirvió %q, quiere v1", body)
	}
	fleet, err := db2.LoadFleet()
	if err != nil || len(fleet.Apps) != 1 || len(fleet.Routes) != 1 {
		t.Fatalf("flota tras reboot: %+v %v", fleet, err)
	}
}

// TestRealBuildRotoConservaViva: syntax error real; el build falla y la v1
// sigue sirviendo en el mismo puerto (sin corte a medias).
func TestRealBuildRotoConservaViva(t *testing.T) {
	py := pythonOrSkip(t)
	repo := fixtureRepo(t, "demo-py")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, live, _, ctl := realWorld(t, py, []string{"-u", "app.py"}, repo, []string{pyCompileStep(py)}, nil)
	if _, err := deploy.RunContext(context.Background(), bbAppOf(live), "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy v1: %v", err)
	}
	p1 := bbPortOf(ctl.live)
	bbWaitServe(t, p1, "v1", 30*time.Second)

	if err := os.WriteFile(filepath.Join(repo, "app.py"), []byte("esto no es python ((((\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "roto")
	_, err := deploy.RunContext(context.Background(), bbAppOf(ctl.live), "", ctl, io.Discard, log)
	if err == nil || !strings.Contains(err.Error(), "py_compile") {
		t.Fatalf("build roto debe fallar en py_compile: %v", err)
	}
	if got := bbPortOf(ctl.live); got != p1 {
		t.Fatalf("puerto cambió sin corte: %q quiere %q", got, p1)
	}
	bbWaitServe(t, p1, "v1", 15*time.Second)
}

// TestRealCanarioMudoNoCorta: proceso vivo que nunca escucha; el deploy
// muere en la espera del canario y no hay corte.
func TestRealCanarioMudoNoCorta(t *testing.T) {
	py := pythonOrSkip(t)
	repo := fixtureRepo(t, "demo-py")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, live, _, ctl := realWorld(t, py, []string{"-c", "import time; time.sleep(60)"}, repo, nil, nil)
	_, err := deploy.RunContext(context.Background(), bbAppOf(live), "", ctl, io.Discard, log)
	if err == nil || !strings.Contains(err.Error(), "canario") {
		t.Fatalf("canario mudo debe fallar: %v", err)
	}
	if got := bbPortOf(ctl.live); got != "1" {
		t.Fatalf("sin corte el puerto sigue placeholder: %q", got)
	}
}

// TestRealFixturesDetectanPlan: buildpack sobre los fixtures reales.
func TestRealFixturesDetectanPlan(t *testing.T) {
	root := fixtureRoot(t)
	py, err := buildpack.DetectDir(filepath.Join(root, "demo-py"))
	if err != nil || py.Kind != "python" {
		t.Fatalf("demo-py: kind=%q err=%v", py.Kind, err)
	}
	if len(py.Start) != 3 || py.Start[0] != "python" || py.Start[2] != "app.py" {
		t.Fatalf("demo-py start=%v", py.Start)
	}
	godir := filepath.Join(root, "demo-go")
	goPlan, err := buildpack.DetectDir(godir)
	if err != nil || goPlan.Kind != "go" {
		t.Fatalf("demo-go: kind=%q err=%v", goPlan.Kind, err)
	}
	if len(goPlan.Build) == 0 || !strings.Contains(goPlan.Build[0], "go build") {
		t.Fatalf("demo-go build=%v", goPlan.Build)
	}
	// Arranque relativo al slot con separador: el supervisor lo resuelve
	// contra el dir del canario (los slots .a/.b rotan; absoluto no sirve).
	wantStart := "." + string(filepath.Separator) + "app"
	if runtime.GOOS == "windows" {
		wantStart += ".exe"
	}
	if len(goPlan.Start) != 1 || goPlan.Start[0] != wantStart {
		t.Fatalf("demo-go start=%v, quiere [%q]", goPlan.Start, wantStart)
	}
	st, err := buildpack.DetectDir(filepath.Join(root, "demo-static"))
	if err != nil || !st.IsStatic() {
		t.Fatalf("demo-static: kind=%q static=%v err=%v", st.Kind, st.IsStatic(), err)
	}
	if st.StaticDir != "." {
		t.Fatalf("demo-static dir=%q, quiere .", st.StaticDir)
	}

	// Familia Node: cada stack con sus archivos reales (sin red ni install).
	nodeCases := []struct {
		dir       string
		kind      string
		start     []string
		staticDir string
		buildSub  string
	}{
		{"demo-node-server", "node", []string{"pnpm", "run", "start"}, "", ""},
		{"demo-react-vite", "vite", nil, "dist", "pnpm run build"},
		{"demo-next", "next", []string{"pnpm", "exec", "next", "start", "-H", "127.0.0.1"}, "", "pnpm run build"},
		{"demo-sveltekit", "sveltekit", []string{"node", "build"}, "", "pnpm run build"},
		{"demo-nuxt", "nuxt", []string{"node", filepath.Join(".output", "server", "index.mjs")}, "", "pnpm run build"},
		{"demo-remix", "remix", []string{"pnpm", "run", "start"}, "", "pnpm run build"},
		{"demo-astro", "astro", nil, "dist", "pnpm run build"},
		{"demo-node-serve", "node", []string{"pnpm", "exec", "serve", "dist"}, "", "pnpm run build"},
		{"demo-node-static", "static", nil, ".", ""},
	}
	for _, c := range nodeCases {
		t.Run(c.dir, func(t *testing.T) {
			p, err := buildpack.DetectDir(filepath.Join(root, c.dir))
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if p.Kind != c.kind {
				t.Fatalf("kind = %q, quiere %q", p.Kind, c.kind)
			}
			if c.start != nil && strings.Join(p.Start, " ") != strings.Join(c.start, " ") {
				t.Fatalf("start = %v, quiere %v", p.Start, c.start)
			}
			if c.staticDir != "" && (!p.IsStatic() || p.StaticDir != c.staticDir) {
				t.Fatalf("static = %q (dir %q), quiere %q", p.Kind, p.StaticDir, c.staticDir)
			}
			if c.buildSub != "" {
				found := false
				for _, s := range p.Build {
					if strings.Contains(s, c.buildSub) {
						found = true
					}
				}
				if !found {
					t.Fatalf("build %v sin %q", p.Build, c.buildSub)
				}
			}
		})
	}
}
