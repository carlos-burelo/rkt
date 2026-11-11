package supervisor

// Regresión: el canario nace dentro de un job creado ANTES del Start, así
// los nietos (pnpm/node, go run, wrappers) quedan dentro y el Kill los
// arrastra. Antes, un nieto nacido antes del Assign tardío escapaba:
// retenía puerto y pipe, y el Stop se colgaba para siempre.

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

// pythonRealForTest binario python de verdad, sin alias de WindowsApps.
// El alias relanza el intérprete mediante un broker fuera del job: los
// límites no aplican y el Kill no lo alcanza (ni a sus nietos). En prod
// se usa el binario real; los tests de contención también deben.
func pythonRealForTest(t *testing.T) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if strings.Contains(strings.ToLower(dir), "windowsapps") {
			continue
		}
		for _, name := range []string{"python.exe", "python3.exe", "python3", "python"} {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err != nil || st.IsDir() {
				continue
			}
			if exec.Command(p, "--version").Run() == nil {
				return p
			}
		}
	}
	t.Skip("sin Python real (solo alias WindowsApps)")
	return ""
}

const canaryParent = `import subprocess, sys, time
pidfile = sys.argv[1]
p = subprocess.Popen([sys.executable, "hijo.py"])
with open(pidfile, "w") as f:
    f.write(str(p.pid))
time.sleep(300)
`

const canaryChild = `import http.server, os
port = int(os.environ.get("PORT", "8080"))
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        b = b"hijo"
        self.send_response(200)
        self.send_header("Content-Length", str(len(b)))
        self.end_headers()
        self.wfile.write(b)
    def log_message(self, *a):
        pass
http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
`

func canaryTreeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "padre.py"), []byte(canaryParent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hijo.py"), []byte(canaryChild), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitServes(t *testing.T, port, want string, d time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://127.0.0.1:" + port + "/")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(b) == want {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("puerto %s no sirvió %q", port, want)
}

func waitRefused(t *testing.T, port string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 500*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("puerto %s siguió abierto (nieto huérfano vivo)", port)
}

// TestCanaryJobMataArbolCompleto: padre (canario) + hijo (servidor real).
// Tras adoptar y parar, el puerto del hijo debe cerrarse y el stop debe
// volver pronto. Sin job-desde-el-inicio, el hijo escapaba: el puerto
// seguía abierto y el Stop se colgaba.
func TestCanaryJobMataArbolCompleto(t *testing.T) {
	py := pythonRealForTest(t)
	dir := canaryTreeDir(t)
	pidfile := filepath.Join(t.TempDir(), "hijo.pid")
	apps := []config.App{app1("arbol", py, []string{"padre.py", pidfile}, 5*time.Second, time.Second)}
	m := New(apps, nil, t.TempDir(), testLog(t))
	port := freePort(t)
	canario, err := m.StartCanary("arbol", dir, port, io.Discard)
	if err != nil {
		t.Fatalf("canario: %v", err)
	}
	// El job queda pendiente hasta la adopción.
	if m.apps[0].canaryJob == nil {
		t.Fatal("StartCanary debe dejar job pendiente de adopción")
	}
	waitServes(t, port, "hijo", 30*time.Second)
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatalf("pidfile: %v", err)
	}
	// Padre y nieto dentro del job (herencia al nacer, no assign tardío).
	a0 := m.apps[0]
	a0.mu.Lock()
	cj := a0.canaryJob
	a0.mu.Unlock()
	if !jobContains(cj, uint32(canario.Process.Pid)) {
		t.Fatal("el canario debe estar en su job desde el arranque")
	}
	if !jobContainsPid(cj, string(b)) {
		t.Fatal("el nieto debe nacer dentro del job del canario")
	}

	if err := m.SwapLive("arbol", canario, dir, port); err != nil {
		t.Fatalf("corte: %v", err)
	}
	waitFor(t, 10*time.Second, func() bool {
		s := m.Snapshot()
		return len(s) == 1 && s[0].Running
	}, "canario adoptado corriendo")
	// Adoptado: el job pendiente se consumió y es el vigente.
	a := m.apps[0]
	a.mu.Lock()
	pendiente, vigente := a.canaryJob, a.job
	a.mu.Unlock()
	if pendiente != nil || vigente == nil {
		t.Fatalf("tras adoptar: pendiente=%v vigente=%v", pendiente != nil, vigente != nil)
	}

	start := time.Now()
	done := make(chan struct{})
	go func() { a.stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(25 * time.Second):
		t.Fatal("stop colgado: nieto fuera del job (pipe retenido)")
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Fatalf("stop tardó %v (límite 20s)", elapsed)
	}
	waitRefused(t, port, 15*time.Second)
	m.Stop()
}

// TestStartCanaryLimpiaJobEnFallo: comando inexistente no deja job pendiente.
func TestStartCanaryLimpiaJobEnFallo(t *testing.T) {
	m := New([]config.App{app1("mala", "definitivamente-no-existe-este-binario-xyz", nil, 5*time.Second, time.Second)}, nil, t.TempDir(), testLog(t))
	if _, err := m.StartCanary("mala", t.TempDir(), "1", io.Discard); err == nil {
		t.Fatal("comando inexistente debe fallar")
	}
	if m.apps[0].canaryJob != nil {
		t.Fatal("fallo de arranque no debe dejar job pendiente")
	}
}
