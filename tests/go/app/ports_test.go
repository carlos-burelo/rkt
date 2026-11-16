package app

import (
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"rkt/internal/cfgfile"
	"rkt/internal/config"
	"rkt/internal/store"
)

func testAppDB(t *testing.T, cfg *config.Config) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := cfgfile.SaveFleet(db, cfg); err != nil {
		t.Fatal(err)
	}
	return db
}

func squat(t *testing.T) (int, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	return p, func() { _ = l.Close() }
}

// Puerto ocupado + app detenida => reubica y retargetea rutas.
func TestEnsurePortsReubicaOcupado(t *testing.T) {
	port, free := squat(t)
	defer free()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg := &config.Config{
		Apps: []config.App{
			{Name: "w", Command: "w.exe", Dir: t.TempDir(), Env: map[string]string{"PORT": strconv.Itoa(port)}},
		},
		Routes: []config.Route{
			{Hosts: []string{"w.test"}, Target: "http://127.0.0.1:" + strconv.Itoa(port)},
		},
	}
	db := testAppDB(t, cfg)
	out, changed := reallocateBusyPorts(db, cfg.Apps, map[string]bool{}, log)
	if !changed {
		t.Fatal("debió reubicar el puerto ocupado")
	}
	got := ""
	for _, a := range out.Apps {
		if a.Name == "w" {
			got = a.Env["PORT"]
		}
	}
	if got == "" || got == strconv.Itoa(port) {
		t.Fatalf("puerto no cambió: %q", got)
	}
	live, err := db.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	var target string
	for _, r := range live.Routes {
		target = r.Target
	}
	if target != "http://127.0.0.1:"+got {
		t.Fatalf("ruta no retargeteada: %s", target)
	}
}

// Puerto libre => sin cambios. Los efímeros pueden chocar con otros
// paquetes en paralelo: reintenta con puertos recién liberados.
func TestEnsurePortsNoTocaLibre(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i := 0; i < 3; i++ {
		p1, rel1 := squat(t)
		p2, rel2 := squat(t)
		rel1()
		rel2()
		cfg := &config.Config{
			Apps: []config.App{
				{Name: "libre", Command: "x.exe", Dir: t.TempDir(), Env: map[string]string{"PORT": strconv.Itoa(p1)}},
				{Name: "otra", Command: "x.exe", Dir: t.TempDir(), Env: map[string]string{"PORT": strconv.Itoa(p2)}},
			},
		}
		db := testAppDB(t, cfg)
		_, changed := reallocateBusyPorts(db, cfg.Apps, map[string]bool{}, log)
		if !changed {
			return
		}
	}
	t.Fatal("puertos libres colisionaron 3 veces seguidas")
}

// En ejecución no se toca aunque su puerto esté ocupado; la detenida sí.
func TestEnsurePortsRespetaEnEjecucion(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// El ocupado-por-otro con app detenida SÍ se reubica; la que corre no
	// se toca aunque su puerto también esté ocupado.
	portQ, releaseQ := squat(t)
	defer releaseQ()
	portV, releaseV := squat(t)
	defer releaseV()
	cfg2 := &config.Config{
		Apps: []config.App{
			{Name: "quieta", Command: "x.exe", Dir: t.TempDir(), Env: map[string]string{"PORT": strconv.Itoa(portQ)}},
			{Name: "viva", Command: "x.exe", Dir: t.TempDir(), Env: map[string]string{"PORT": strconv.Itoa(portV)}},
		},
		Routes: []config.Route{
			{Hosts: []string{"quieta.test"}, Target: "http://127.0.0.1:" + strconv.Itoa(portQ)},
			{Hosts: []string{"viva.test"}, Target: "http://127.0.0.1:" + strconv.Itoa(portV)},
		},
	}
	db2 := testAppDB(t, cfg2)
	out2, changed := reallocateBusyPorts(db2, cfg2.Apps, map[string]bool{"viva": true}, log)
	if !changed {
		t.Fatal("debió reubicar la detenida")
	}
	byName := map[string]config.App{}
	for _, a := range out2.Apps {
		byName[a.Name] = a
	}
	if byName["viva"].Env["PORT"] != strconv.Itoa(portV) {
		t.Fatalf("la app en ejecución se movió: %+v", byName["viva"].Env)
	}
	if byName["quieta"].Env["PORT"] == strconv.Itoa(portQ) {
		t.Fatal("la detenida no se movió")
	}
	live, err := db2.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range live.Routes {
		if len(r.Hosts) == 1 && r.Hosts[0] == "viva.test" && r.Target != "http://127.0.0.1:"+strconv.Itoa(portV) {
			t.Fatalf("ruta ajena tocada: %s", r.Target)
		}
		if len(r.Hosts) == 1 && r.Hosts[0] == "quieta.test" && r.Target == "http://127.0.0.1:"+strconv.Itoa(portQ) {
			t.Fatalf("ruta no retargeteada: %s", r.Target)
		}
	}
}
