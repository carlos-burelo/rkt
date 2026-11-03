package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

// faultDB abre una DB y aplica SQL de prueba (triggers/vistas) para forzar
// las ramas de error que no ocurren con datos válidos.
func faultDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exec(t *testing.T, db *DB, sqlText string) {
	t.Helper()
	if _, err := db.sql.Exec(sqlText); err != nil {
		t.Fatalf("sql %q: %v", sqlText, err)
	}
}

func TestScanErroresConVistas(t *testing.T) {
	cases := []struct {
		name   string
		rename string
		view   string
		call   func(*DB) error
	}{
		{"users", "users", `CREATE VIEW users AS SELECT name, role, NULL AS created_at FROM users_real`, func(db *DB) error {
			_, err := db.ListUsers()
			return err
		}},
		{"alerts", "alert_rules", `CREATE VIEW alert_rules AS SELECT id, NULL AS app, kind, threshold, window_min, last_fired, created_at FROM alert_rules_real`, func(db *DB) error {
			_, err := db.ListRules()
			return err
		}},
		{"apps", "apps", `CREATE VIEW apps AS SELECT name, runtime, command, NULL AS args, dir, env_file, env, restart_delay_ms, min_uptime_ms, memory_mb, deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_auto FROM apps_real`, func(db *DB) error {
			_, err := db.LoadFleet()
			return err
		}},
		{"routes", "routes", `CREATE VIEW routes AS SELECT NULL AS hosts, target, id FROM routes_real`, func(db *DB) error {
			_, err := db.LoadFleet()
			return err
		}},
		{"deployments", "deployments", `CREATE VIEW deployments AS SELECT id, NULL AS app, rev, status, started_at, finished_at, triggered_by, env_snapshot FROM deployments_real`, func(db *DB) error {
			_, err := db.ListDeployments("", 10)
			return err
		}},
		{"events", "events", `CREATE VIEW events AS SELECT id, ts, NULL AS kind, app, msg FROM events_real`, func(db *DB) error {
			_, err := db.ListEvents(10, "", "")
			return err
		}},
		{"infra", "infra", `CREATE VIEW infra AS SELECT id, NULL AS data, updated_at FROM infra_real`, func(db *DB) error {
			_, err := db.LoadInfra()
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := faultDB(t)
			// datos mínimos para que la consulta devuelva filas
			switch c.name {
			case "users":
				_ = db.CreateUser("u1", "password-larga-1", RoleViewer)
			case "alerts":
				_, _ = db.AddRule("a", "failed", 1, 60)
			case "apps":
				_ = db.UpsertApp(&config.App{Name: "a", Command: "x", Env: map[string]string{}})
			case "routes":
				_, _ = db.AddRoute([]string{"a.test"}, "http://127.0.0.1:1")
			case "deployments":
				_, _ = db.InsertDeployment("a", "r", "", "", nil)
			case "events":
				_ = db.InsertEvent("died", "a", "m")
			case "infra":
				_ = db.SaveInfra([]byte("{}"))
			}
			exec(t, db, `ALTER TABLE `+c.rename+` RENAME TO `+c.rename+`_real`)
			exec(t, db, c.view)
			if err := c.call(db); err == nil {
				t.Fatalf("%s: el Scan debía fallar", c.name)
			}
		})
	}
}

func TestTriggersDeError(t *testing.T) {
	// routes INSERT (AddRoute e ImportFleet)
	db := faultDB(t)
	exec(t, db, `CREATE TRIGGER no_routes BEFORE INSERT ON routes BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if _, err := db.AddRoute([]string{"a.test"}, "http://127.0.0.1:1"); err == nil {
		t.Fatal("AddRoute debía fallar por trigger")
	}
	if err := db.ImportFleet(&config.Config{
		Apps:   []config.App{{Name: "a", Command: "x"}},
		Routes: []config.Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:1"}},
	}); err == nil {
		t.Fatal("ImportFleet ruta debía fallar por trigger")
	}

	// apps INSERT
	db = faultDB(t)
	exec(t, db, `CREATE TRIGGER no_apps_ins BEFORE INSERT ON apps BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.ImportFleet(&config.Config{Apps: []config.App{{Name: "a", Command: "x"}}}); err == nil {
		t.Fatal("ImportFleet app debía fallar por trigger")
	}

	// apps DELETE
	db = faultDB(t)
	_ = db.UpsertApp(&config.App{Name: "a", Command: "x"})
	exec(t, db, `CREATE TRIGGER no_apps_del BEFORE DELETE ON apps BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.ImportFleet(&config.Config{}); err == nil {
		t.Fatal("ImportFleet delete apps debía fallar")
	}

	// routes DELETE
	db = faultDB(t)
	_, _ = db.AddRoute([]string{"a.test"}, "http://127.0.0.1:1")
	exec(t, db, `CREATE TRIGGER no_routes_del BEFORE DELETE ON routes BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.ImportFleet(&config.Config{}); err == nil {
		t.Fatal("ImportFleet delete routes debía fallar")
	}

	// meta INSERT (bumpVersion)
	db = faultDB(t)
	exec(t, db, `CREATE TRIGGER no_meta BEFORE INSERT ON meta BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.SetSetting("k", "v"); err == nil {
		t.Fatal("SetSetting debía fallar en bump")
	}

	// infra INSERT (SaveInfra)
	db = faultDB(t)
	exec(t, db, `CREATE TRIGGER no_infra BEFORE INSERT ON infra BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.SaveInfra([]byte("{}")); err == nil {
		t.Fatal("SaveInfra debía fallar por trigger")
	}

	// users DELETE/UPDATE
	db = faultDB(t)
	_ = db.CreateUser("u1", "password-larga-1", RoleViewer)
	_ = db.CreateUser("u2", "password-larga-1", RoleViewer)
	exec(t, db, `CREATE TRIGGER no_users_del BEFORE DELETE ON users BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.DeleteUser("u2"); err == nil {
		t.Fatal("DeleteUser debía fallar por trigger")
	}
	db = faultDB(t)
	_ = db.CreateUser("u1", "password-larga-1", RoleViewer)
	exec(t, db, `CREATE TRIGGER no_users_upd BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.SetPassword("u1", "password-larga-2"); err == nil {
		t.Fatal("SetPassword debía fallar por trigger")
	}
}

func TestBcryptMuyLargoYRandFalla(t *testing.T) {
	db := faultDB(t)
	long := strings.Repeat("x", 100)
	if err := db.CreateUser("u1", long, RoleViewer); err == nil {
		t.Fatal("bcrypt debe rechazar password >72 bytes")
	}
	_ = db.CreateUser("u2", "password-larga-1", RoleViewer)
	if err := db.SetPassword("u2", long); err == nil {
		t.Fatal("SetPassword bcrypt debe rechazar >72 bytes")
	}
	// rand.Read fallando (hook)
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("sin entropía") }
	t.Cleanup(func() { randRead = old })
	if _, err := db.NewSession("u2", time.Hour); err == nil {
		t.Fatal("NewSession debía fallar sin entropía")
	}
}

func TestOpenYMigrateErrores(t *testing.T) {
	oldOpen := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, errors.New("sin driver") }
	t.Cleanup(func() { sqlOpen = oldOpen })
	if _, err := Open(filepath.Join(t.TempDir(), "x.db")); err == nil {
		t.Fatal("Open debía fallar si sql.Open falla")
	}
	sqlOpen = oldOpen

	// sql.Open devuelve una DB ya cerrada: los PRAGMA fallan
	closed, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	sqlOpen = func(string, string) (*sql.DB, error) { return closed, nil }
	if _, err := Open(filepath.Join(t.TempDir(), "y.db")); err == nil {
		t.Fatal("Open debía fallar en los PRAGMA")
	}
	sqlOpen = oldOpen

	// migrate con DB cerrada (PRAGMA user_version falla)
	db := faultDB(t)
	_ = db.Close()
	if err := db.migrate(); err == nil {
		t.Fatal("migrate cerrada debía fallar")
	}

	// migrate sobre DB read-only con user_version=0: el Exec de la migración falla
	path := filepath.Join(t.TempDir(), "ro.db")
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db2.sql.Exec(`PRAGMA user_version=0`); err != nil {
		t.Fatal(err)
	}
	db2.Close()
	ro, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if err := (&DB{sql: ro}).migrate(); err == nil {
		t.Fatal("migrate read-only debía fallar")
	}
}

func TestRamasRestantes(t *testing.T) {
	// SaveInfra con receptor nil
	var nilDB *DB
	if err := nilDB.SaveInfra([]byte("{}")); err == nil {
		t.Fatal("SaveInfra nil debía fallar")
	}

	// LoadInfra sin fila => nil, nil
	db := faultDB(t)
	if data, err := db.LoadInfra(); err != nil || data != nil {
		t.Fatalf("LoadInfra vacía = %q %v", data, err)
	}

	// LoadFleet sin tabla routes => error de query
	db = faultDB(t)
	exec(t, db, `DROP TABLE routes`)
	if _, err := db.LoadFleet(); err == nil {
		t.Fatal("LoadFleet sin routes debía fallar")
	}

	// AddRoute: fallo al subir versión (meta)
	db = faultDB(t)
	exec(t, db, `CREATE TRIGGER no_meta3 BEFORE INSERT ON meta BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if _, err := db.AddRoute([]string{"a.test"}, "http://127.0.0.1:1"); err == nil {
		t.Fatal("AddRoute debía fallar en bumpVersion")
	}

	// ImportFleet: fallo en la fila de meta
	db = faultDB(t)
	exec(t, db, `CREATE TRIGGER no_meta4 BEFORE INSERT ON meta BEGIN SELECT RAISE(ABORT,'no'); END;`)
	if err := db.ImportFleet(&config.Config{}); err == nil {
		t.Fatal("ImportFleet debía fallar en meta")
	}

	// Open: la migración falla si ya existe una vista con el nombre de tabla
	path := filepath.Join(t.TempDir(), "mig.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	exec(t, first, `DROP TABLE apps`)
	exec(t, first, `CREATE VIEW apps AS SELECT 1 AS x`)
	exec(t, first, `PRAGMA user_version=0`)
	first.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("Open debía fallar si la migración choca con una vista")
	}
}

// TestLoadFleetRowsErr fuerza un error a mitad de iteración (json inválido
// en la segunda fila) para cubrir rows.Err.
func TestLoadFleetRowsErr(t *testing.T) {
	db := faultDB(t)
	_ = db.UpsertApp(&config.App{Name: "a", Command: "x", Env: map[string]string{}})
	_ = db.UpsertApp(&config.App{Name: "b", Command: "x", Env: map[string]string{}})
	exec(t, db, `ALTER TABLE apps RENAME TO apps_real`)
	exec(t, db, `CREATE VIEW apps AS SELECT name, runtime, command,
		CASE WHEN name='b' THEN json_extract('no-json','$') ELSE args END AS args,
		dir, env_file, env, restart_delay_ms, min_uptime_ms, memory_mb,
		deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_auto
		FROM apps_real ORDER BY name`)
	if _, err := db.LoadFleet(); err == nil {
		t.Fatal("LoadFleet debía fallar al iterar (rows.Err)")
	}
}
