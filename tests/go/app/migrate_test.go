package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

// migrateSlots corre en cada arranque: convierte Dir sin sufijo a <name>.a.
// Sin test, una regresión dejaría apps viejas fuera del blue-green.
func TestMigrateSlots(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	legacy := filepath.Join(dir, "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps, config.App{
			Name: "legacy", Command: "c", Dir: legacy,
			Env:          map[string]string{"PORT": "3210"},
			RestartDelay: "5s", MinUptime: "10s",
		})
		cp.Routes = append(cp.Routes, config.Route{
			Hosts: []string{"legacy.test"}, Target: "http://127.0.0.1:3210",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	migrated, err := migrateSlots(db, cp)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if migrated == nil {
		t.Fatal("debió migrar el layout antiguo")
	}
	got := cfgfile.FindApp(migrated, "legacy")
	if got == nil || !strings.HasSuffix(got.Dir, ".a") {
		t.Fatalf("Dir sin sufijo .a: %+v", got)
	}
	if _, err := os.Stat(legacy + ".a"); err != nil {
		t.Fatalf("dir renombrado: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("dir viejo debe desaparecer")
	}

	// idempotente: segunda pasada no toca nada
	again, err := migrateSlots(db, migrated)
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	if again != nil {
		t.Fatal("segunda pasada debe ser no-op")
	}
}
