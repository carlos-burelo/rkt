package cfgfile

import (
	"os"
	"path/filepath"
	"testing"

	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func openDB(t *testing.T, cfgPath string) *store.DB {
	t.Helper()
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSeedDevYPersistencia(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "rkt.dev.yaml")
	db := openDB(t, cfgPath)
	if err := SeedDev(db, "8091"); err != nil {
		t.Fatal(err)
	}
	base, err := LoadBase(db, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if base.ListenHTTP != ":8091" || base.ACMEEmail != "dev@localhost" {
		t.Fatalf("infra dev = %+v", base)
	}
	// segunda carga sin YAML: misma infra desde DB
	db2 := openDB(t, cfgPath)
	base2, err := LoadBase(db2, cfgPath)
	if err != nil || base2.ListenHTTP != ":8091" {
		t.Fatalf("recarga = %+v %v", base2, err)
	}
}

func TestMigraYAMLUnaVez(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	yaml := "listen_http: \":9090\"\nacme_email: \"a@b.c\"\nhealthz:\n  enabled: true\n  listen: \"127.0.0.1:8099\"\nroutes:\n  - hosts: [\"x.test\"]\n    target: \"http://127.0.0.1:3000\"\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	db := openDB(t, cfgPath)
	base, err := LoadBase(db, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if base.ListenHTTP != ":9090" {
		t.Fatalf("infra migrada = %+v", base)
	}
	if _, err := os.Stat(cfgPath + ".migrated"); err != nil {
		t.Fatal("el YAML debía quedar como .migrated")
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatal("el YAML no debe seguir en su nombre original")
	}
	// la flota se importó una sola vez
	fleet, err := db.LoadFleet()
	if err != nil || len(fleet.Routes) != 1 {
		t.Fatalf("fleet = %+v %v", fleet, err)
	}
}
