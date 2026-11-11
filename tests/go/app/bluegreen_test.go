package app

// Integración del pipeline blue-green de punta a punta, sin stubs:
// repo git real, build real, canario real (proceso python con HTTP),
// corte real (Live.Swap + adopción del supervisor) y rollback real.
// Esto es la esencia de los 4 flujos: deploy sync, deploy async,
// build local y rollback.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/supervisor"
)

func pythonOrSkip(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		if path, err := exec.LookPath(name); err == nil {
			if exec.Command(path, "--version").Run() == nil {
				return path
			}
		}
	}
	t.Skip("sin Python ejecutable")
	return ""
}

const bbServer = `import http.server, os
VERSION = %q
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers()
        self.wfile.write(VERSION.encode())
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()
`

// mkRepoBB crea repo git con app.py en la versión dada. Devuelve el dir.
func mkRepoBB(t *testing.T, version string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte(fmt.Sprintf(bbServer, version)), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", version, "--no-gpg-sign")
	return dir
}

func bbCommit(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte(fmt.Sprintf(bbServer, version)), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", version, "--no-gpg-sign"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

var bbClient = &http.Client{Timeout: 15 * time.Second}

func bbGet(t *testing.T, port string) (string, error) {
	t.Helper()
	resp, err := bbClient.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func bbPortOf(live *config.Live) string {
	for _, a := range live.Get().Apps {
		if a.Name == "bb" {
			return a.Env["PORT"]
		}
	}
	return ""
}

func bbAppOf(live *config.Live) config.App {
	for _, a := range live.Get().Apps {
		if a.Name == "bb" {
			return a
		}
	}
	return config.App{}
}

// bbWorld monta DB + Live + supervisor reales con la app bb.
func bbWorld(t *testing.T, py, repo string, build []string) (*store.DB, *config.Live, *supervisor.Manager, *supCtl) {
	t.Helper()
	base := t.TempDir()
	db, err := store.Open(filepath.Join(base, "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps, config.App{
			Name: "bb", Command: py, Args: []string{"-u", "app.py"},
			Dir: filepath.Join(base, "bb.a"), Env: map[string]string{"PORT": "1"},
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
	// El martillo pasaría el freno (20rps): se desactiva en infra para que
	// los MutateFleet del corte lo conserven, y en la copia viva actual.
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

func bbWaitServe(t *testing.T, port, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if body, err := bbGet(t, port); err == nil && body == want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("puerto %s no sirvió %q", port, want)
}

// hammer martilla el proxy (Host bb.test) hasta cerrar stop. Devuelve las
// respuestas (status+body) en orden y cuántos requests fallaron. Los errores
// de lectura cuentan como fallo (tragarlos fabricaba bodies "" fantasma);
// el timeout del cliente es generoso (15s): una caída real (refused, 404,
// 502) responde de inmediato, así que no enmascara downtime.
func hammer(t *testing.T, proxyURL string, stop <-chan struct{}) (samples func() []probe, errs func() int) {
	t.Helper()
	var mu sync.Mutex
	var got []probe
	var nerr int
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			req, _ := http.NewRequest("GET", proxyURL, nil)
			req.Host = "bb.test"
			resp, err := bbClient.Do(req)
			mu.Lock()
			if err != nil {
				nerr++
			} else {
				b, rerr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if rerr != nil {
					nerr++
				} else {
					got = append(got, probe{status: resp.StatusCode, body: string(b)})
				}
			}
			mu.Unlock()
		}
	}()
	return func() []probe {
			mu.Lock()
			defer mu.Unlock()
			return append([]probe(nil), got...)
		}, func() int {
			mu.Lock()
			defer mu.Unlock()
			return nerr
		}
}

// probe una respuesta del martillo con su status (para diagnosticar
// contenidos inesperados: "" con 200 no es lo mismo que un 502).
type probe struct {
	status int
	body   string
}

// assertCorteLimpio exige: cero errores, solo from/to, ambos vistos y una
// sola transición from* to* (sin flapping ni huecos).
func assertCorteLimpio(t *testing.T, samples []probe, nerr int, from, to string) {
	t.Helper()
	if nerr != 0 {
		t.Fatalf("corte con %d requests caídos de %d", nerr, len(samples)+nerr)
	}
	if len(samples) == 0 {
		t.Fatal("el martillo no registró respuestas")
	}
	seenFrom, seenTo := false, false
	flipped := false
	for _, p := range samples {
		switch p.body {
		case from:
			seenFrom = true
			if flipped {
				t.Fatalf("flapping: volvió %q tras %q", from, to)
			}
		case to:
			seenTo = true
			flipped = true
		default:
			t.Fatalf("contenido inesperado %q (status %d, quiere %q/%q; muestra: %s)",
				p.body, p.status, from, to, sampleStr(samples))
		}
	}
	if !seenFrom || !seenTo {
		t.Fatalf("sin transición completa %q->%q (v1=%v v2=%v, n=%d; muestra: %s)", from, to, seenFrom, seenTo, len(samples), sampleStr(samples))
	}
}

// sampleStr primeras respuestas (status+body) para diagnosticar en CI.
func sampleStr(samples []probe) string {
	var sb strings.Builder
	for i, p := range samples {
		if i >= 5 {
			break
		}
		fmt.Fprintf(&sb, "[%d %q] ", p.status, p.body)
	}
	return sb.String()
}

// Deploy sync con cero downtime real: mientras el build+canario corren,
// la versión vieja sigue respondiendo; tras el corte solo la nueva.
func TestBlueGreenZeroDowntimeReal(t *testing.T) {
	py := pythonOrSkip(t)
	repo := mkRepoBB(t, "v1")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// build de ~6s: sin cancelación en este test, los hijos terminan solos.
	slow := "sleep 6"
	if runtime.GOOS == "windows" {
		slow = "ping -n 7 127.0.0.1 >nul"
	}

	_, live, _, ctl := bbWorld(t, py, repo, []string{"echo build-v1"})
	app := bbAppOf(live)
	rev1, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log)
	if err != nil {
		t.Fatalf("deploy v1: %v", err)
	}
	if rev1 == "" || rev1 == "?" {
		t.Fatalf("rev vacío: %q", rev1)
	}
	p1 := bbPortOf(live)
	bbWaitServe(t, p1, "v1", 20*time.Second)

	// v2 con build lento: el corte tarda, la v1 no puede caer.
	bbCommit(t, repo, "v2")
	cp, err := cfgfile.MutateFleet(ctl.db, func(cp *config.Config) error {
		return cfgfile.UpdateApp(cp, "bb", cfgfile.AppPatch{Deploy: &cfgfile.DeployPatch{
			Repo: repo, Ref: "main", Build: []string{slow},
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	ctl.live.Swap(cp)

	done := make(chan string, 1)
	// Martillo contra el PROXY (no contra los puertos): la ruta bb.test
	// retargeta en el Live.Swap y el corte debe ser invisible.
	front := httptest.NewServer(proxy.NewHandler(ctl.live, log))
	defer front.Close()
	stop := make(chan struct{})
	samples, errs := hammer(t, front.URL, stop)
	go func() {
		rev, err := deploy.RunContext(context.Background(), bbAppOf(ctl.live), "", ctl, io.Discard, log)
		if err != nil {
			done <- "ERR:" + err.Error()
			return
		}
		done <- rev
	}()
	select {
	case r := <-done:
		close(stop)
		if strings.HasPrefix(r, "ERR:") {
			t.Fatalf("deploy v2: %s", r)
		}
		if r == "" || r == "?" {
			t.Fatalf("rev vacío: %q", r)
		}
	case <-time.After(120 * time.Second):
		close(stop)
		t.Fatal("deploy v2 no terminó")
	}
	// Cero downtime real: ningún request al proxy cayó y el contenido pasó
	// de v1 a v2 en una sola transición (sin huecos ni flapping).
	assertCorteLimpio(t, samples(), errs(), "v1", "v2")
	p2 := bbPortOf(live)
	if p2 == "" || p2 == p1 {
		t.Fatalf("corte sin puerto nuevo: p1=%q p2=%q", p1, p2)
	}
	bbWaitServe(t, p2, "v2", 20*time.Second)
	if _, err := bbGet(t, p1); err == nil {
		t.Fatal("la instancia vieja sigue viva tras el corte")
	}

	// rollback martillando el proxy: vuelve v2->v1 sin caídas.
	stopRb := make(chan struct{})
	samplesRb, errsRb := hammer(t, front.URL, stopRb)
	if err := deploy.Rollback(bbAppOf(live), ctl, log); err != nil {
		close(stopRb)
		t.Fatalf("rollback: %v", err)
	}
	close(stopRb)
	assertCorteLimpio(t, samplesRb(), errsRb(), "v2", "v1")
	p3 := bbPortOf(live)
	if p3 == "" || p3 == p2 {
		t.Fatalf("rollback sin puerto nuevo: p2=%q p3=%q", p2, p3)
	}
	bbWaitServe(t, p3, "v1", 20*time.Second)
	if _, err := bbGet(t, p2); err == nil {
		t.Fatal("la v2 sigue viva tras el rollback")
	}
}

// tailDeployLog vuelca la cola del log de build más reciente: en CI el
// estado "failed" sin el log es ciego (pip vs restart vs health).
func tailDeployLog(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) == 0 {
		return "(sin logs en " + dir + ")"
	}
	name := ents[len(ents)-1].Name() // orden léxico = cronológico (timestamp)
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "(ilegible " + name + ": " + err.Error() + ")"
	}
	if len(b) > 4096 {
		b = b[len(b)-4096:]
	}
	return "[" + name + "]\n" + string(b)
}

// Deploy async real vía deployAppAsync: devuelve el id al instante y el
// pipeline corre en fondo hasta dejar la app sirviendo.
func TestBlueGreenAsyncReal(t *testing.T) {
	py := pythonOrSkip(t)
	repo := mkRepoBB(t, "v1")
	// build de ~8s para probar que el async devuelve al instante.
	slow := "sleep 8"
	if runtime.GOOS == "windows" {
		slow = "ping -n 9 127.0.0.1 >nul"
	}
	base := t.TempDir()
	cfgPath := filepath.Join(base, "rkt.db")
	db, err := store.Open(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps, config.App{
			Name: "bb", Command: py, Args: []string{"-u", "app.py"},
			Dir: filepath.Join(base, "bb.a"), Env: map[string]string{"PORT": "1"},
			RestartDelay: "5s", MinUptime: "1s",
			Deploy: &config.Deploy{Repo: repo, Ref: "main", Build: []string{slow}},
		})
		cp.Routes = append(cp.Routes, config.Route{
			Hosts: []string{"bb.test"}, Target: "http://127.0.0.1:1",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	p := New(cfgPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.db = db
	p.live = config.NewLive(cp)
	p.sup = supervisor.New(cp.Apps, nil, filepath.Join(base, "logs"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer p.sup.Stop()

	start := time.Now()
	id, err := p.deployAppAsync("bb", "tester")
	if err != nil {
		t.Fatalf("async: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id inválido: %d", id)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("async no devolvió al instante: %v", elapsed)
	}
	deadline := time.Now().Add(120 * time.Second)
	for {
		st, ok := db.DeploymentStatus(id)
		if ok && st != "running" {
			if st != "ok" {
				t.Fatalf("deploy async terminó en %q", st)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deploy async no terminó")
		}
		time.Sleep(500 * time.Millisecond)
	}
	var port string
	for _, a := range p.live.Get().Apps {
		if a.Name == "bb" {
			port = a.Env["PORT"]
		}
	}
	bbWaitServe(t, port, "v1", 20*time.Second)
}

// Build local real: compila en el dir vivo (app parada), reinicia y la app
// vuelve a responder. Requiere pip (requirements vacío, sin red).
func TestBlueGreenBuildLocalReal(t *testing.T) {
	py := pythonOrSkip(t)
	if _, err := exec.LookPath("pip"); err != nil {
		if _, err2 := exec.LookPath("pip3"); err2 != nil {
			t.Skip("sin pip")
		}
	}
	base := t.TempDir()
	liveDir := filepath.Join(base, "bb.a")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "app.py"), []byte(fmt.Sprintf(bbServer, "v0")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "requirements.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(base, "rkt.db")
	db, err := store.Open(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps, config.App{
			Name: "bb", Command: py, Args: []string{"-u", "app.py"},
			Dir: liveDir, Env: map[string]string{"PORT": "1"},
			RestartDelay: "5s", MinUptime: "1s",
		})
		cp.Routes = append(cp.Routes, config.Route{
			Hosts: []string{"bb.test"}, Target: "http://127.0.0.1:1",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	p := New(cfgPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.db = db
	p.live = config.NewLive(cp)
	p.sup = supervisor.New(cp.Apps, nil, filepath.Join(base, "logs"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer p.sup.Stop()
	p.sup.Start()
	deadline := time.Now().Add(30 * time.Second)
	for {
		running := false
		for _, s := range p.sup.Snapshot() {
			if s.Name == "bb" && s.Running {
				running = true
			}
		}
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("la app no arrancó")
		}
		time.Sleep(200 * time.Millisecond)
	}

	id, err := p.buildLocal("bb", "tester")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id inválido: %d", id)
	}
	deadline = time.Now().Add(120 * time.Second)
	for {
		st, ok := db.DeploymentStatus(id)
		if ok && st != "running" {
			if st != "ok" {
				t.Fatalf("build terminó en %q\n--- deploy log ---\n%s", st, tailDeployLog(filepath.Join(base, "logs", "deploys")))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("build no terminó")
		}
		time.Sleep(500 * time.Millisecond)
	}
	var port string
	for _, a := range p.live.Get().Apps {
		if a.Name == "bb" {
			port = a.Env["PORT"]
		}
	}
	// el build no cambia código: la app responde lo mismo tras reiniciar.
	bbWaitServe(t, port, "v0", 20*time.Second)
}
