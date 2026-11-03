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

func TestApplyDevYPersistencia(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "rkt.dev.yaml")
	db := openDB(t, cfgPath)
	if err := ApplyDev(db, "127.0.0.1:80", "127.0.0.1:443"); err != nil {
		t.Fatal(err)
	}
	base, err := LoadBase(db, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if base.ListenHTTP != "127.0.0.1:80" || base.ListenHTTPS != "127.0.0.1:443" {
		t.Fatalf("listeners dev = %q %q", base.ListenHTTP, base.ListenHTTPS)
	}
	if !base.LocalCA || !base.RedirectHTTPToHTTPS {
		t.Fatalf("dev debe usar CA local y redirigir 80→443: %+v", base)
	}
	// reaplicar con otros puertos no se ignora aunque la DB ya exista
	if err := ApplyDev(db, "127.0.0.1:8080", ""); err != nil {
		t.Fatal(err)
	}
	base2, err := LoadBase(db, cfgPath)
	if err != nil || base2.ListenHTTP != "127.0.0.1:8080" || base2.ListenHTTPS != "" {
		t.Fatalf("reaplicar = %+v %v", base2, err)
	}
	if base2.LocalCA || base2.RedirectHTTPToHTTPS {
		t.Fatalf("sin https no debe haber CA local ni redirect: %+v", base2)
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
