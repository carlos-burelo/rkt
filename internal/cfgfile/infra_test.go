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

func TestIgnoraYAMLViejo(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("listen_http: \":9090\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db := openDB(t, cfgPath)
	base, err := LoadBase(db, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// SQLite es la única fuente: el archivo no se lee ni se renombra
	if base.ListenHTTP != ":80" {
		t.Fatalf("infra = %q, quiere default :80", base.ListenHTTP)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatal("el archivo viejo no debe tocarse")
	}
	if _, err := os.Stat(cfgPath + ".migrated"); !os.IsNotExist(err) {
		t.Fatal("no debe existir .migrated")
	}
	fleet, err := db.LoadFleet()
	if err != nil || len(fleet.Routes) != 0 {
		t.Fatalf("fleet = %+v %v", fleet, err)
	}
}

func TestSetGlobalEnvPersistencia(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "rkt.dev.yaml")
	db := openDB(t, cfgPath)
	if _, err := LoadBase(db, cfgPath); err != nil {
		t.Fatal(err)
	}
	// expansión: B referencia a A, A referencia el entorno del servicio
	t.Setenv("RKT_TEST_BASE", "base")
	if err := SetGlobalEnv(db, "A", "${RKT_TEST_BASE}-a"); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalEnv(db, "B", "${A}-b"); err != nil {
		t.Fatal(err)
	}
	reload, err := LoadBase(db, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if reload.GlobalEnv["A"] != "base-a" || reload.GlobalEnv["B"] != "base-a-b" {
		t.Fatalf("globales = %+v", reload.GlobalEnv)
	}
	if err := UnsetGlobalEnv(db, "A"); err != nil {
		t.Fatal(err)
	}
	reload, err = LoadBase(db, cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reload.GlobalEnv["A"]; ok {
		t.Fatalf("A debió borrarse: %+v", reload.GlobalEnv)
	}
	if reload.GlobalEnv["B"] != "base-a-b" {
		t.Fatalf("B no debe cambiar: %+v", reload.GlobalEnv)
	}
}
