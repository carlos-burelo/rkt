package pg

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

// memReg Registry en memoria para probar la orquestación sin SQLite.
type memReg struct {
	apps map[string]Record
}

func newMemReg() *memReg { return &memReg{apps: map[string]Record{}} }

func (m *memReg) PgGet(app string) (Record, bool, error) {
	r, ok := m.apps[app]
	return r, ok, nil
}
func (m *memReg) PgList() ([]Record, error) {
	out := []Record{}
	for _, r := range m.apps {
		out = append(out, r)
	}
	return out, nil
}
func (m *memReg) PgPut(app, schema, role, mode, db string) error {
	m.apps[app] = Record{App: app, Schema: schema, Role: role, Mode: mode, DB: db}
	return nil
}
func (m *memReg) PgRemove(app string) error {
	delete(m.apps, app)
	return nil
}

// TestPgLiveOrquesta ciclo completo contra Postgres real: provision
// (esquema+rol+trust), conexión como rol app sin password, drop y
// limpieza. Solo con RKT_PG_TEST_DSN (superusuario local) y
// RKT_PG_TEST_PASS. En CI se salta.
func TestPgLiveOrquesta(t *testing.T) {
	db := liveSuper(t)
	defer db.Close()
	ctx := context.Background()

	cfg := Config{
		Host: "127.0.0.1", Port: 5432, User: "postgres",
		Pass: os.Getenv("RKT_PG_TEST_PASS"), DB: "rkt_test_int",
	}

	hba, err := hbaPath(t, db)
	if err != nil {
		t.Skipf("sin hba_file: %v", err)
	}
	orig, err := os.ReadFile(hba)
	if err != nil {
		t.Fatalf("leer hba: %v", err)
	}
	defer func() {
		_ = os.WriteFile(hba, orig, 0o644)
		_, _ = db.ExecContext(context.Background(), `SELECT pg_reload_conf()`)
		_, _ = db.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+QuoteIdent("rkt_test_int"))
	}()

	reg := newMemReg()
	if err := EnsureDatabase(ctx, db, cfg.DB); err != nil {
		t.Fatalf("ensure db: %v", err)
	}

	const app = "rkt_test_web"
	url, err := ProvisionApp(ctx, reg, cfg, app, "shared")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	rec, ok, err := reg.PgGet(app)
	if err != nil || !ok || rec.Mode != "shared" || rec.Role != SharedRole {
		t.Fatalf("registro: %+v %v %v", rec, ok, err)
	}

	// El rol app conecta sin password gracias a la regla trust.
	appDB, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer appDB.Close()
		ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := appDB.PingContext(ctx2); err != nil {
			t.Fatalf("rol app sin trust: %v", err)
		}
		if _, err := appDB.ExecContext(ctx2, `CREATE TABLE t1 (id int primary key)`); err != nil {
			t.Fatalf("ddl en esquema propio: %v", err)
		}
	}()

	if err := DropApp(ctx, reg, cfg, app); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, ok, _ := reg.PgGet(app); ok {
		t.Fatal("registro debió borrarse")
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname='rkt_test_web'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("esquema no borrado: %v", err)
	}
}

func hbaPath(t *testing.T, db *sql.DB) (string, error) {
	t.Helper()
	var hba string
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.QueryRowContext(ctx, `SHOW hba_file`).Scan(&hba); err != nil {
		return "", err
	}
	if hba == "" {
		return "", fmt.Errorf("hba_file vacío")
	}
	return hba, nil
}
