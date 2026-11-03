package appusage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
)

func TestHostsForMapeaPorPuerto(t *testing.T) {
	cfg := &config.Config{Routes: []config.Route{
		{Hosts: []string{"Sicesa.localhost"}, Target: "http://127.0.0.1:3002"},
		{Hosts: []string{"otra.test"}, Target: "http://127.0.0.1:3001"},
	}}
	app := config.App{Name: "sicesa", Env: map[string]string{"PORT": "3002"}}
	hosts := hostsFor(cfg, app)
	if !hosts["sicesa.localhost"] || len(hosts) != 1 {
		t.Fatalf("hosts = %+v", hosts)
	}
	if hostsFor(cfg, config.App{Name: "sin-puerto"}) != nil {
		t.Fatal("sin PORT no hay hosts")
	}
}

func TestDirSizeYFileSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.bin"), []byte("1234567890"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, seen := dirSize(dir)
	if got != 15 || !seen {
		t.Fatalf("dirSize = %d seen=%v, quiere 15 true", got, seen)
	}
	if fileSize(filepath.Join(dir, "a.txt")) != 5 {
		t.Fatal("fileSize mal")
	}
	if fileSize(filepath.Join(dir, "no-existe")) != 0 {
		t.Fatal("archivo inexistente debe ser 0")
	}
}

func TestTrafficFiltraHostYVentana(t *testing.T) {
	logs := t.TempDir()
	now := time.Now()
	line := func(host string, when time.Time, status, n int) string {
		return fmt.Sprintf("1.2.3.4 - - [%s] \"GET / HTTP/1.1\" %d %d \"-\" \"ua\" host=%q dur=1ms\n",
			when.Format("02/Jan/2006:15:04:05 -0700"), status, n, host)
	}
	content := line("sicesa.localhost", now, 200, 100) +
		line("sicesa.localhost", now, 500, 50) +
		line("otra.test", now, 200, 999) +
		line("sicesa.localhost", now.Add(-time.Hour), 200, 777)
	if err := os.WriteFile(filepath.Join(logs, "access.log"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Routes: []config.Route{{Hosts: []string{"sicesa.localhost"}, Target: "http://127.0.0.1:3002"}}}
	in := Input{
		App:     config.App{Name: "sicesa", Env: map[string]string{"PORT": "3002"}},
		Cfg:     cfg,
		LogsDir: logs,
		Status:  health.AppStatus{Name: "sicesa"},
		Window:  5 * time.Minute,
	}
	rep, err := Collect(in)
	if err != nil {
		t.Fatal(err)
	}
	tr := rep.Traffic
	if tr.Requests != 2 || tr.Bytes != 150 || tr.Errors != 1 {
		t.Fatalf("traffic = %+v", tr)
	}
	if tr.ErrPct != 50 || tr.RPS <= 0 {
		t.Fatalf("porcentajes = %+v", tr)
	}
	if rep.Running {
		t.Fatal("sin pid debe reportar detenida, no inventar")
	}
}
