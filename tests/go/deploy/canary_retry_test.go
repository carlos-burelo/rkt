package deploy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"rkt/internal/config"
	"rkt/internal/supervisor"
)

type fakeCanary struct {
	calls int
	ports []string
}

func (f *fakeCanary) SwapAndRestart(name string, swap func() error) error { return swap() }
func (f *fakeCanary) Snapshot() []supervisor.Status                       { return nil }
func (f *fakeCanary) StartCanary(name, dir, port string, out io.Writer) (*exec.Cmd, error) {
	f.calls++
	f.ports = append(f.ports, port)
	cmd := exec.Command("cmd", "/C", "exit 1")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
func (f *fakeCanary) Cutover(name string, cmd *exec.Cmd, dir, port string) error { return nil }

// Si el puerto elegido lo ocupa otro al arrancar, reintenta con puerto
// fresco en vez de fallar el deploy.
func TestStartCanaryReintentaPuertoOcupado(t *testing.T) {
	oldWait, oldEphem := canaryWait, ephemeralPortFn
	canaryWait = 500 * time.Millisecond
	defer func() { canaryWait, ephemeralPortFn = oldWait, oldEphem }()

	squat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer squat.Close()
	// primer puerto = el ocupado; resto = libres reales.
	first := true
	ephemeralPortFn = func() (string, error) {
		if first {
			first = false
			_, p, err := net.SplitHostPort(squat.Listener.Addr().String())
			return p, err
		}
		return freeEphemeralPort()
	}
	fc := &fakeCanary{}
	app := config.App{Name: "w", Dir: t.TempDir()}
	_, port, err := startCanaryChecked(fc, app, app.Dir, io.Discard)
	if err == nil {
		t.Fatal("debió fallar (canario muerto en puerto libre)")
	}
	if fc.calls != 2 {
		t.Fatalf("llamadas = %d, quiere 2 (1 ocupado + 1 libre)", fc.calls)
	}
	if len(fc.ports) != 2 || fc.ports[0] == fc.ports[1] {
		t.Fatalf("puertos sin rotar: %v", fc.ports)
	}
	if port != "" {
		t.Fatalf("puerto = %q, quiere vacío en fallo", port)
	}
}

// Sin ocupación no hay reintento: 1 llamada y error del probe.
func TestStartCanarySinReintentoSiLibre(t *testing.T) {
	oldWait := canaryWait
	canaryWait = 500 * time.Millisecond
	defer func() { canaryWait = oldWait }()
	fc := &fakeCanary{}
	app := config.App{Name: "w", Dir: t.TempDir()}
	_, _, err := startCanaryChecked(fc, app, app.Dir, io.Discard)
	if err == nil {
		t.Fatal("debió fallar (canario muerto)")
	}
	if fc.calls != 1 {
		t.Fatalf("llamadas = %d, quiere 1", fc.calls)
	}
}
