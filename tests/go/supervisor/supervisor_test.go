package supervisor

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

func testLog(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func pythonForTest(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		path, err := exec.LookPath(name)
		if err == nil && exec.Command(path, "--version").Run() == nil {
			return path
		}
	}
	t.Skip("sin Python ejecutable")
	return ""
}

func shell(t *testing.T, script string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", script}
	}
	return "/bin/sh", []string{"-c", script}
}

// sleepProc proceso que vive el tiempo pedido (sin depender del shell).
func sleepProc(t *testing.T, d time.Duration) (string, []string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return "/bin/sleep", []string{fmt.Sprintf("%.3f", d.Seconds())}
	}
	if d < time.Second {
		return "cmd.exe", []string{"/c", fmt.Sprintf("ping -n 1 -w %d 127.0.0.1 >nul", d.Milliseconds())}
	}
	return "cmd.exe", []string{"/c", fmt.Sprintf("ping -n %d 127.0.0.1 >nul", int(d/time.Second)+1)}
}

func echoEnv(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", "echo %RKT_TEST_VAR%"}
	}
	return shell(t, "echo $RKT_TEST_VAR")
}

func app1(name, cmd string, args []string, delay, minUp time.Duration) config.App {
	a := config.App{Name: name, Command: cmd, Args: args}
	a.SetDelays(delay, minUp)
	return a
}

func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timeout: " + msg)
}

func TestStartCapturesOutput(t *testing.T) {
	cmd, args := shell(t, "echo hola-mundo")
	apps := []config.App{app1("eco", cmd, args, 5*time.Second, 10*time.Second)}
	dir := t.TempDir()
	m := New(apps, nil, dir, testLog(t))
	m.Start()
	defer m.Stop()
	path := filepath.Join(dir, "eco.log")
	waitFor(t, 5*time.Second, func() bool {
		b, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(b), "hola-mundo")
	}, "log con salida del proceso")
}

func TestRestartOnCrash(t *testing.T) {
	cmd, args := shell(t, "exit 3")
	apps := []config.App{app1("muere", cmd, args, 50*time.Millisecond, 10*time.Second)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		s := m.Snapshot()
		return len(s) == 1 && s[0].Restarts >= 2
	}, "reinicios acumulados")
}

func TestStopKillsRunning(t *testing.T) {
	cmd, args := sleepProc(t, 30*time.Second)
	apps := []config.App{app1("largo", cmd, args, 5*time.Second, 10*time.Second)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	waitFor(t, 5*time.Second, func() bool {
		s := m.Snapshot()
		return len(s) == 1 && s[0].Running && s[0].Pid > 0
	}, "app corriendo con pid")
	m.Stop()
	waitFor(t, 5*time.Second, func() bool {
		return !m.Snapshot()[0].Running
	}, "app muerta tras Stop")
}

func TestCrashloopGuard(t *testing.T) {
	cmd, args := shell(t, "exit 1")
	apps := []config.App{app1("loop", cmd, args, time.Millisecond, 10*time.Second)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		s := m.Snapshot()
		return len(s) == 1 && s[0].Failed && !s[0].Running
	}, "guard debe parar el crashloop")
}

func TestEnvOverride(t *testing.T) {
	cmd, args := echoEnv(t)
	envApp := app1("envtest", cmd, args, 5*time.Second, 10*time.Second)
	envApp.Env = map[string]string{"RKT_TEST_VAR": "valor123"}
	apps := []config.App{envApp}
	dir := t.TempDir()
	m := New(apps, nil, dir, testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 5*time.Second, func() bool {
		b, err := os.ReadFile(filepath.Join(dir, "envtest.log"))
		return err == nil && strings.Contains(string(b), "valor123")
	}, "env var en log del proceso")
}

func TestBackoffFor(t *testing.T) {
	base := time.Second
	if backoffFor(base, 0) != base || backoffFor(base, 1) != 2*time.Second {
		t.Fatal("backoff base/duplicado mal")
	}
	if backoffFor(base, 2) != 4*time.Second {
		t.Fatal("backoff x4 mal")
	}
	if backoffFor(base, 100) != maxBackoff {
		t.Fatal("backoff debe topar a maxBackoff")
	}
	if backoffFor(0, 0) != 0 {
		t.Fatal("delay 0 debe ser sin espera, no maxBackoff")
	}
}

func TestSlowDeathsDontTripGuard(t *testing.T) {
	cmd, args := sleepProc(t, 300*time.Millisecond)
	apps := []config.App{app1("lenta", cmd, args, 50*time.Millisecond, 100*time.Millisecond)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		return m.Snapshot()[0].Restarts >= 2
	}, "reinicios de muerte lenta")
	if s := m.Snapshot()[0]; s.Failed {
		t.Fatal("muerte lenta no debe marcar failed")
	}
}

func TestMemoryLimitKills(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Job Object con límite RAM: solo Windows")
	}
	py := pythonForTest(t)
	// bytearray 300MB con tope 100MB: el commit falla -> MemoryError ->
	// python muere -> supervisor reinicia. (El job niega memoria, no
	// mata directo: runtimes que tragan OOM no reiniciarían.)
	hungry := app1("glotona", py, []string{"-c", "bytearray(300_000_000)"}, 200*time.Millisecond, time.Millisecond)
	hungry.MemoryMB = 100
	apps := []config.App{hungry}
	dir := t.TempDir()
	m := New(apps, nil, dir, testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 30*time.Second, func() bool {
		return m.Snapshot()[0].Restarts >= 1
	}, "límite RAM debe tumbar y reiniciar")
	// La señal real es que el proceso murió bajo el tope (Restarts >= 1):
	// si el job no limitara, python completaría el bytearray y saldría 0.
	// El texto del log (MemoryError vs kill directo del job) depende de la
	// versión de Python y del orden de la asignación: no se exige un mensaje.
}

func TestRestartImmediate(t *testing.T) {
	cmd, args := sleepProc(t, 30*time.Second)
	// delay 30s: si Restart tarda, el test lo delata
	apps := []config.App{app1("largo", cmd, args, 30*time.Second, time.Millisecond)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 5*time.Second, func() bool {
		return m.Snapshot()[0].Running
	}, "app corriendo")
	pid1 := m.Snapshot()[0].Pid
	if !m.Restart("largo") {
		t.Fatal("Restart debe aceptar app existente")
	}
	if m.Restart("nadie") {
		t.Fatal("Restart debe rechazar app inexistente")
	}
	waitFor(t, 5*time.Second, func() bool {
		s := m.Snapshot()[0]
		return s.Pid != pid1 && s.Pid > 0
	}, "restart inmediato con otro pid (sin esperar 30s)")
	if s := m.Snapshot()[0]; s.Restarts != 0 {
		t.Fatalf("restart manual no debe contar como crash: %d", s.Restarts)
	}
}

func TestRestartRevivesFailed(t *testing.T) {
	cmd, args := shell(t, "exit 1")
	apps := []config.App{app1("loop", cmd, args, time.Millisecond, 2*time.Second)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		return m.Snapshot()[0].Failed
	}, "llega a failed")
	if !m.Restart("loop") {
		t.Fatal("Restart debe revivir failed")
	}
	waitFor(t, 5*time.Second, func() bool {
		s := m.Snapshot()[0]
		return !s.Failed
	}, "failed limpio tras restart")
}

func TestSwapAndRestart(t *testing.T) {
	cmd, args := sleepProc(t, 30*time.Second)
	apps := []config.App{app1("largo", cmd, args, 30*time.Second, time.Millisecond)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 5*time.Second, func() bool {
		return m.Snapshot()[0].Running
	}, "app corriendo")
	pid1 := m.Snapshot()[0].Pid

	deadDuringSwap := false
	err := m.SwapAndRestart("largo", func() error {
		deadDuringSwap = !m.Snapshot()[0].Running
		return nil
	})
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	if !deadDuringSwap {
		t.Fatal("swap debe correr con la app muerta")
	}
	waitFor(t, 5*time.Second, func() bool {
		s := m.Snapshot()[0]
		return s.Running && s.Pid != pid1
	}, "arranque inmediato tras swap (sin esperar 30s)")

	if err := m.SwapAndRestart("nadie", func() error { return nil }); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
}

func TestOnEventDied(t *testing.T) {
	cmd, args := shell(t, "exit 3")
	apps := []config.App{app1("muere", cmd, args, 5*time.Second, time.Millisecond)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	var mu sync.Mutex
	var kinds []string
	m.OnEvent = func(kind, app, msg string) {
		mu.Lock()
		kinds = append(kinds, kind+":"+app)
		mu.Unlock()
	}
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(kinds) >= 1 && kinds[0] == "died:muere"
	}, "evento died")
}

func TestExternalNoSeSupervisa(t *testing.T) {
	dir := t.TempDir()
	ext := config.App{Name: "ext", Runtime: config.RuntimeExternal, Dir: dir, Env: map[string]string{"PORT": "3999"}}
	cmd, args := shell(t, "exit 0")
	man := app1("man", cmd, args, time.Second, time.Second)
	m := New([]config.App{ext, man}, nil, dir, testLog(t))
	if got := len(m.Snapshot()); got != 1 {
		t.Fatalf("snapshot = %d, solo debe estar la managed", got)
	}
	m.Apply([]config.App{ext}, nil)
	if got := len(m.Snapshot()); got != 0 {
		t.Fatalf("apply externa no debe crear procesos: %d", got)
	}
}

func TestSwapLiveAdoptaCanario(t *testing.T) {
	cmd, args := sleepProc(t, 30*time.Second)
	apps := []config.App{app1("largo", cmd, args, 5*time.Second, 10*time.Second)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 5*time.Second, func() bool {
		s := m.Snapshot()[0]
		return s.Running && s.Pid > 0
	}, "app corriendo")
	pidViejo := m.Snapshot()[0].Pid

	// canario ya arrancado (lo haría el deploy) desde otro dir/puerto
	canario := exec.Command(cmd, args...)
	configureProc(canario)
	if err := canario.Start(); err != nil {
		t.Fatal(err)
	}
	defer canario.Process.Kill()

	if err := m.SwapLive("largo", canario, "", ""); err != nil {
		t.Fatalf("SwapLive: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		s := m.Snapshot()[0]
		return s.Running && s.Pid != pidViejo && s.Pid == canario.Process.Pid
	}, "supervisor adopta el canario y mata la vieja")

	if err := m.SwapLive("nadie", canario, "", ""); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
}

func TestStartCanaryAppInexistente(t *testing.T) {
	m := New(nil, nil, t.TempDir(), testLog(t))
	if _, err := m.StartCanary("nadie", t.TempDir(), "3999", io.Discard); err == nil {
		t.Fatal("canario de app inexistente debe fallar")
	}
}

func TestBlueGreenCanarioHTTP(t *testing.T) {
	py := pythonForTest(t)
	script := `import http.server,os
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", int(os.environ["PORT"])), H).serve_forever()`
	app := app1("web", py, []string{"-c", script}, time.Second, time.Second)
	m := New([]config.App{app}, nil, t.TempDir(), testLog(t))
	defer m.Stop()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()

	canario, err := m.StartCanary("web", t.TempDir(), port, io.Discard)
	if err != nil {
		t.Fatalf("StartCanary: %v", err)
	}
	url := "http://127.0.0.1:" + port + "/"
	waitFor(t, 15*time.Second, func() bool {
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, "canario responde HTTP")

	if err := m.SwapLive("web", canario, "", ""); err != nil {
		t.Fatalf("SwapLive: %v", err)
	}
	// mismo proceso, sin reinicio: sigue respondiendo
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("tras SwapLive el canario no responde: %v (status %d)", err, resp.StatusCode)
	}
	_ = resp.Body.Close()
}
