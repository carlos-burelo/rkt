package pg

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

// TestPgLiveFull corre contra un Postgres real solo si RKT_PG_TEST_DSN
// apunta al superusuario (local del dev: postgres://postgres:root@127.0.0.1:5432/postgres).
// En CI se salta (sin servidor).
func liveSuper(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("RKT_PG_TEST_DSN")
	if dsn == "" {
		t.Skip("sin RKT_PG_TEST_DSN: se omite integración real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("sin driver pg: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("sin postgres local: %v", err)
	}
	return db
}

func TestPgLiveFull(t *testing.T) {
	db := liveSuper(t)
	defer db.Close()
	ctx := context.Background()
	const dbName = "rkt_test_int"
	const schema = "rkt_test_app"

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP DATABASE IF EXISTS `+QuoteIdent(dbName))
	})

	if err := EnsureDatabase(ctx, db, dbName); err != nil {
		t.Fatalf("ensure db: %v", err)
	}
	if err := EnsureDatabase(ctx, db, dbName); err != nil {
		t.Fatalf("ensure idempotente: %v", err)
	}

	p, err := PlanProvision(schema, "dedicated", dbName, "127.0.0.1", 5432)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyProvision(ctx, db, p); err != nil {
		t.Fatalf("provision: %v", err)
	}
	// Repetir es idempotente.
	if err := ApplyProvision(ctx, db, p); err != nil {
		t.Fatalf("re-provision: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname=$1`, schema).Scan(&n); err != nil || n != 1 {
		t.Fatalf("esquema no creado: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_roles WHERE rolname=$1`, schema).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rol no creado: %v", err)
	}
	// search_path fijado para el rol en esa DB.
	var sp string
	q := `SELECT unnest(setconfig) FROM pg_db_role_setting WHERE setrole = (SELECT oid FROM pg_roles WHERE rolname=$1) AND setdatabase = (SELECT oid FROM pg_database WHERE datname=$2)`
	if err := db.QueryRowContext(ctx, q, schema, dbName).Scan(&sp); err != nil || sp != "search_path="+schema {
		t.Fatalf("search_path = %q, %v", sp, err)
	}

	if err := DropAll(ctx, db, schema, schema, "dedicated"); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname=$1`, schema).Scan(&n); err != nil || n != 0 {
		t.Fatalf("esquema no borrado: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_roles WHERE rolname=$1`, schema).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rol no borrado: %v", err)
	}
}
