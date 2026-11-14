package cfgfile

// Migración al hogar único: DB/backups del layout viejo se mueven a data/,
// los certs del default viejo al home. Idempotente y sin pisar.

import (
	"os"
	"path/filepath"
	"testing"

	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func TestMigrateLegacyHomeMueveDB(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "rkt.db") // ancla del home
	legacyDB := filepath.Join(base, "logs", "rkt.db")
	db, err := store.Open(legacyDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(store.KeySiteName, "migra"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "logs", "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "logs", "backups", "rkt-vieja.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// certs del home ya creado: no debe tocar el default real de la máquina.
	if err := os.MkdirAll(filepath.Join(base, "certs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyHome(cfg); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyHome(cfg); err != nil {
		t.Fatalf("reentrada: %v", err)
	}

	if _, err := os.Stat(legacyDB); !os.IsNotExist(err) {
		t.Fatal("la DB vieja debe moverse")
	}
	db2, err := store.Open(logx.DBPath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if got := db2.GetSetting(store.KeySiteName); got != "migra" {
		t.Fatalf("dato migrado = %q", got)
	}
	if _, err := os.Stat(filepath.Join(base, "data", "backups", "rkt-vieja.db")); err != nil {
		t.Fatalf("backup no migrado: %v", err)
	}
	// LoadMerged sobre el layout nuevo abre sin migrar nada más.
	if _, db3, err := LoadMerged(cfg); err != nil {
		t.Fatal(err)
	} else {
		db3.Close()
	}
}

func TestMigrateLegacyHomeEnFrescoNoTocaNada(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.db")
	if err := MigrateLegacyHome(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logx.DBPath(cfg)); !os.IsNotExist(err) {
		t.Fatal("en fresco no debe crear nada")
	}
}

func TestCopyDirArbol(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copia")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "sub", "a.txt")); err != nil || string(b) != "a" {
		t.Fatalf("copia = %q %v", b, err)
	}
}
