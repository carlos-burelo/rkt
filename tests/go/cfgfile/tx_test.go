package cfgfile

import (
	"path/filepath"
	"sync"
	"testing"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

// Dos handles distintos (como dos procesos) mutan campos distintos a la vez.
// BEGIN IMMEDIATE debe serializar: ninguna escritura pierde el cambio ajeno.
func TestMutateFleetConcurrenteSinLostUpdate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rkt.db")
	db1, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db1.Close()
	app := config.App{
		Name: "w", Command: "x", Dir: dir,
		Env:          map[string]string{"PORT": "3001"},
		RestartDelay: "5s", MinUptime: "10s",
	}
	if err := AddApp(db1, app, nil, ""); err != nil {
		t.Fatal(err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	delay, uptime := "9s", "20s"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := MutateFleet(db1, func(cp *config.Config) error {
			return UpdateApp(cp, "w", AppPatch{RestartDelay: &delay})
		})
		errs <- err
	}()
	go func() {
		defer wg.Done()
		_, err := MutateFleet(db2, func(cp *config.Config) error {
			return UpdateApp(cp, "w", AppPatch{MinUptime: &uptime})
		})
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	fleet, err := db1.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	if len(fleet.Apps) != 1 {
		t.Fatalf("apps = %d", len(fleet.Apps))
	}
	got := fleet.Apps[0]
	if got.RestartDelay != delay || got.MinUptime != uptime {
		t.Fatalf("lost update: restart=%q min_uptime=%q", got.RestartDelay, got.MinUptime)
	}
}
