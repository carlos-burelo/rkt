package store

// Upgrade real: DB creada en una versión vieja (solo migraciones[0:k],
// user_version=k) con datos de época; Open() migra al día y los datos
// sobreviven. Es exactamente lo que pasa al actualizar cada beta.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func oldHash(t *testing.T) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte("secreto-largo-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

// buildOldDB aplica migraciones[0:k], fija user_version=k y siembra datos
// con el esquema de la época (columnas que existían entonces).
func buildOldDB(t *testing.T, k int, seed func(t *testing.T, db *sql.DB)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vieja.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < k; i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			db.Close()
			t.Fatalf("migración %d: %v", i+1, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", k)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	seed(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func seedV10(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO apps(name,command,args,dir,env,restart_delay_ms,min_uptime_ms,memory_mb,deploy_repo,deploy_ref,deploy_token_env,deploy_build,deploy_auto,runtime)
		VALUES('web','python','["-u","app.py"]','C:\apps\web','{"PORT":"8080"}',5000,10000,0,'https://github.com/o/r.git','main','RKT_TOK','[]',1,'managed')`); err != nil {
		t.Fatalf("seed app v10: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO routes(hosts,target) VALUES('["web.test"]','http://127.0.0.1:8080')`); err != nil {
		t.Fatalf("seed route v10: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('base_domain','coatl.in')`); err != nil {
		t.Fatalf("seed setting v10: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users(name,pass_hash,role,created_at) VALUES('ana',?,'operator','2026-01-01')`, oldHash(t)); err != nil {
		t.Fatalf("seed user v10: %v", err)
	}
	// v7 ya aportaba triggered_by + env_snapshot; v15 la quitará.
	if _, err := db.Exec(`INSERT INTO deployments(app,rev,prev_rev,status,started_at,finished_at,triggered_by,env_snapshot)
		VALUES('web','abc1234','','ok','2026-01-01','2026-01-01','tester','{}')`); err != nil {
		t.Fatalf("seed deployment v10: %v", err)
	}
}

func seedV18(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO apps(name,command,args,dir,env,restart_delay_ms,min_uptime_ms,memory_mb,deploy_repo,deploy_ref,deploy_token_env,deploy_build,deploy_auto,runtime,disabled,health_path,static,cron,persist)
		VALUES('web','python','["-u","app.py"]','C:\apps\web','{"PORT":"8080"}',5000,10000,0,'https://github.com/o/r.git','main','RKT_TOK','[]',1,'managed',0,'/salud','','[]','["data"]')`); err != nil {
		t.Fatalf("seed app v18: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO routes(hosts,target) VALUES('["web.test"]','http://127.0.0.1:8080')`); err != nil {
		t.Fatalf("seed route v18: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users(name,pass_hash,role,created_at,updated_at,last_login,disabled) VALUES('ana',?,'admin','2026-01-01','','',0)`, oldHash(t)); err != nil {
		t.Fatalf("seed user v18: %v", err)
	}
	// env_snapshot ya no existe desde v15.
	if _, err := db.Exec(`INSERT INTO deployments(app,rev,prev_rev,status,started_at,finished_at,triggered_by)
		VALUES('web','def5678','abc1234','ok','2026-02-01','2026-02-01','hook')`); err != nil {
		t.Fatalf("seed deployment v18: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO api_tokens(name,token_hash,role,created_at,expires_at,last_used) VALUES('ci','h','operator','','','')`); err != nil {
		t.Fatalf("seed token v18: %v", err)
	}
}

func assertMigrada(t *testing.T, path, wantRev, wantRole string) {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open tras upgrade: %v", err)
	}
	defer db.Close()

	var v int
	if err := db.sql.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Fatalf("user_version=%d, quiere %d", v, len(migrations))
	}
	fleet, err := db.LoadFleet()
	if err != nil {
		t.Fatalf("flota: %v", err)
	}
	if len(fleet.Apps) != 1 || fleet.Apps[0].Name != "web" || fleet.Apps[0].Dir != `C:\apps\web` {
		t.Fatalf("app migrada = %+v", fleet.Apps)
	}
	if fleet.Apps[0].Deploy == nil || fleet.Apps[0].Deploy.Repo != "https://github.com/o/r.git" || fleet.Apps[0].Deploy.Ref != "main" {
		t.Fatalf("deploy migrado = %+v", fleet.Apps[0].Deploy)
	}
	if len(fleet.Routes) != 1 || len(fleet.Routes[0].Hosts) != 1 || fleet.Routes[0].Hosts[0] != "web.test" {
		t.Fatalf("rutas migradas = %+v", fleet.Routes)
	}
	if got := db.GetSetting("base_domain"); got != "" && got != "coatl.in" {
		t.Fatalf("setting base_domain = %q", got)
	}
	users, err := db.ListUsers()
	if err != nil || len(users) != 1 || users[0].Name != "ana" || users[0].Role != wantRole {
		t.Fatalf("usuarios migrados = %+v %v", users, err)
	}
	deps, err := db.ListDeployments("web", 10)
	if err != nil || len(deps) != 1 || deps[0].Rev != wantRev || deps[0].Status != "ok" {
		t.Fatalf("deployments migrados = %+v %v", deps, err)
	}
	// Escritura con el esquema nuevo sobre la DB migrada.
	id, err := db.InsertDeployment("web", wantRev, "", "tester-post")
	if err != nil || id <= 0 {
		t.Fatalf("insert post-upgrade: %v %d", err, id)
	}
	if err := db.FinishDeployment(id, "ok", wantRev, "rev-nueva"); err != nil {
		t.Fatalf("finish post-upgrade: %v", err)
	}

	// Reapertura idempotente.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	var v2 int
	if err := db2.sql.QueryRow("PRAGMA user_version").Scan(&v2); err != nil || v2 != len(migrations) {
		t.Fatalf("reopen version=%d err=%v", v2, err)
	}
}

// Camino largo: v10 -> actual (pasa por DROP installations, audit,
// api_tokens, persist, deploy_release, basic_auth... y el DROP COLUMN de
// env_snapshot en v15).
func TestUpgradeDesdeV10(t *testing.T) {
	path := buildOldDB(t, 10, seedV10)
	assertMigrada(t, path, "abc1234", "operator")
}

// Camino corto: v18 -> actual (lo típico entre betas seguidas).
func TestUpgradeDesdeV18(t *testing.T) {
	path := buildOldDB(t, 18, seedV18)
	assertMigrada(t, path, "def5678", "admin")
}
