package cronjobs

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

func TestRunNowYHistorial(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "cron.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := &config.Config{
		Apps: []config.App{{
			Name: "a", Command: "x", Dir: dir, RestartDelay: "1s", MinUptime: "1s",
			Env: map[string]string{"PORT": "3000"},
			Cron: []config.CronJob{{
				Name: "eco", Schedule: "* * * * *", Command: "echo cron-ok", Timeout: "1m",
			}},
		}},
	}
	live := config.NewLive(cfg)
	r := New(live, db, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	id, err := r.RunNow("a", "eco")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, ok, err := db.CronRunByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if ok && run.Status != "running" {
			if run.Status != "ok" || run.LogPath == "" {
				t.Fatalf("run = %+v", run)
			}
			b, err := os.ReadFile(run.LogPath)
			if err != nil || !strings.Contains(string(b), "cron-ok") {
				t.Fatalf("log = %q %v", b, err)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("cron no terminó")
}

func TestRunNowValidaciones(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "cron.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := &config.Config{Apps: []config.App{{
		Name: "a", Command: "x", Dir: dir, RestartDelay: "1s", MinUptime: "1s",
		Cron: []config.CronJob{{Name: "off", Schedule: "* * * * *", Command: "echo", Disabled: true}},
	}}}
	r := New(config.NewLive(cfg), db, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := r.RunNow("nope", "x"); err == nil {
		t.Fatal("app inexistente")
	}
	if _, err := r.RunNow("a", "nope"); err == nil {
		t.Fatal("job inexistente")
	}
	if _, err := r.RunNow("a", "off"); err == nil {
		t.Fatal("job desactivado")
	}
}
