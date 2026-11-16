package store

import (
	"path/filepath"
	"testing"
)

func TestPgProvisionCRUD(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.PgProvision("", "s", "r", "dedicated", "rkt"); err == nil {
		t.Fatal("app vacía debe fallar")
	}
	if err := db.PgProvision("w", "s", "r", "otro", "rkt"); err == nil {
		t.Fatal("mode inválido debe fallar")
	}
	if err := db.PgProvision("web", "web", "web", "dedicated", "rkt"); err != nil {
		t.Fatal(err)
	}
	if err := db.PgProvision("web", "web", "web", "dedicated", "rkt"); err == nil {
		t.Fatal("duplicado debe fallar")
	}
	if err := db.PgProvision("api", "api", "rkt_apps", "shared", "rkt"); err != nil {
		t.Fatal(err)
	}

	got, ok, err := db.PgGet("web")
	if err != nil || !ok {
		t.Fatalf("get: %+v %v %v", got, ok, err)
	}
	if got.Schema != "web" || got.Role != "web" || got.Mode != "dedicated" || got.DB != "rkt" || got.CreatedAt == "" {
		t.Fatalf("mapeo: %+v", got)
	}
	if _, ok, err := db.PgGet("nadie"); err != nil || ok {
		t.Fatalf("inexistente: %v %v", ok, err)
	}

	list, err := db.PgList()
	if err != nil || len(list) != 2 || list[0].App != "api" || list[1].App != "web" {
		t.Fatalf("list ordenada: %+v %v", list, err)
	}

	if err := db.PgRemove("nadie"); err == nil {
		t.Fatal("borrar inexistente debe fallar")
	}
	if err := db.PgRemove("web"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.PgGet("web"); ok {
		t.Fatal("debió borrarse")
	}
}
