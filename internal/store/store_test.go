package store

import (
	"path/filepath"
	"testing"
)

func TestMigrateIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rkt.db")
	db, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db2, err := Open(p) // reabre: migraciones no repiten
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
}

func TestDeploymentsRoundtrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.InsertDeployment("web", "", "/tmp/x.log")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployment(id, "ok", "prevrev", "newrev"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertDeployment("web", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployment(id+1, "failed", "", ""); err != nil {
		t.Fatal(err)
	}

	all, err := db.ListDeployments("", 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("list: %v %+v", err, all)
	}
	if all[0].Status != "failed" || all[1].Rev != "newrev" || all[1].Status != "ok" {
		t.Fatalf("orden/contenido mal: %+v", all)
	}
	mine, err := db.ListDeployments("web", 1)
	if err != nil || len(mine) != 1 || mine[0].ID != all[0].ID {
		t.Fatalf("filtro app mal: %+v %v", mine, err)
	}
	other, err := db.ListDeployments("otra", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("app inexistente: %+v %v", other, err)
	}
}

func TestEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.InsertEvent("died", "web", "exit 1"); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertEvent("died", "web", "exit 2"); err != nil {
		t.Fatal(err)
	}
	n, err := db.CountEvents("died")
	if err != nil || n != 2 {
		t.Fatalf("count: %d %v", n, err)
	}
	n, _ = db.CountEvents("nada")
	if n != 0 {
		t.Fatalf("count vacío: %d", n)
	}
}
