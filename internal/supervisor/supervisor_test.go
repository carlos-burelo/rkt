package supervisor

import (
	"io"
	"log/slog"
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

func shell(t *testing.T, script string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", script}
	}
	return "sh", []string{"-c", script}
}

func loadApps(t *testing.T, yamlBody string) []config.App {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-sup-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString("routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n" + yamlBody); err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg.Apps
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
	apps := loadApps(t, "apps:\n  - name: eco\n    command: \""+cmd+"\"\n    args: [\""+strings.Join(args, "\", \"")+"\"]\n    restart_delay: 5s\n")
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
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: muere\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 50ms\n")
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		s := m.Snapshot()
		return len(s) == 1 && s[0].Restarts >= 2
	}, "reinicios acumulados")
}

func TestStopKillsRunning(t *testing.T) {
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd, args = "cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 >nul"}
	} else {
		cmd, args = "sleep", []string{"30"}
	}
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + strings.ReplaceAll(a, `"`, `\"`) + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: largo\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 5s\n")
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
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: loop\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 1ms\n")
	m := New(apps, nil, t.TempDir(), testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 10*time.Second, func() bool {
		s := m.Snapshot()
		return len(s) == 1 && s[0].Failed && !s[0].Running
	}, "guard debe parar el crashloop")
}

func TestEnvOverride(t *testing.T) {
	var cmd, script string
	var args []string
	if runtime.GOOS == "windows" {
		cmd, args = "cmd.exe", []string{"/c", "echo %RKT_TEST_VAR%"}
		_ = script
	} else {
		cmd, args = "sh", []string{"-c", "echo $RKT_TEST_VAR"}
	}
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: envtest\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    env:\n      RKT_TEST_VAR: valor123\n    restart_delay: 5s\n")
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
}

func TestSlowDeathsDontTripGuard(t *testing.T) {
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd, args = "cmd.exe", []string{"/c", "ping -n 1 -w 300 127.0.0.1 >nul"}
	} else {
		cmd, args = "sleep", []string{"0.3"}
	}
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	// muere cada ~300ms pero min_uptime 100ms: reinicia sin contar crash
	apps := loadApps(t, "apps:\n  - name: lenta\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 50ms\n    min_uptime: 100ms\n")
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
		t.Skip("límite RAM vía Job Objects es Windows")
	}
	py, err := exec.LookPath("python")
	if err != nil {
		py, err = exec.LookPath("python3")
	}
	if err != nil {
		t.Skip("sin python para la prueba")
	}
	// bytearray 300MB con tope 100MB: el commit falla -> MemoryError ->
	// python muere -> supervisor reinicia. (El job niega memoria, no
	// mata directo: runtimes que tragan OOM no reiniciarían.)
	apps := loadApps(t, "apps:\n  - name: glotona\n    command: '"+py+"'\n    args: [\"-c\", \"bytearray(300_000_000)\"]\n    restart_delay: 200ms\n    min_uptime: 1ms\n    memory_mb: 100\n")
	dir := t.TempDir()
	m := New(apps, nil, dir, testLog(t))
	m.Start()
	defer m.Stop()
	waitFor(t, 30*time.Second, func() bool {
		return m.Snapshot()[0].Restarts >= 1
	}, "límite RAM debe tumbar y reiniciar")
	b, _ := os.ReadFile(filepath.Join(dir, "glotona.log"))
	if !strings.Contains(string(b), "MemoryError") {
		t.Fatalf("el log debe mostrar OOM, no otra muerte: %q", trunc(string(b), 300))
	}
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func TestRestartImmediate(t *testing.T) {
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd, args = "cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 >nul"}
	} else {
		cmd, args = "sleep", []string{"30"}
	}
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	// delay 30s: si Restart tarda, el test lo delata
	apps := loadApps(t, "apps:\n  - name: largo\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 30s\n    min_uptime: 1ms\n")
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
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: loop\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 1ms\n    min_uptime: 2s\n")
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
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd, args = "cmd.exe", []string{"/c", "ping -n 30 127.0.0.1 >nul"}
	} else {
		cmd, args = "sleep", []string{"30"}
	}
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: largo\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 30s\n    min_uptime: 1ms\n")
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
	var argYaml string
	for _, a := range args {
		argYaml += "\"" + a + "\", "
	}
	apps := loadApps(t, "apps:\n  - name: muere\n    command: \""+cmd+"\"\n    args: ["+argYaml+"]\n    restart_delay: 5s\n    min_uptime: 1ms\n")
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
