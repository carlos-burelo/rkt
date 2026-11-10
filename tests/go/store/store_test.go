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

	id, err := db.InsertDeployment("web", "", "/tmp/x.log", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployment(id, "ok", "prevrev", "newrev"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertDeployment("web", "", "", "tester"); err != nil {
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
	if all[1].TriggeredBy != "tester" {
		t.Fatalf("trazabilidad mal: %+v", all[1])
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

func TestListEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.InsertEvent("died", "a", "x1")
	db.InsertEvent("failed", "a", "x2")
	db.InsertEvent("died", "b", "x3")

	all, err := db.ListEvents(10, "", "")
	if err != nil || len(all) != 3 || all[0].Kind != "died" || all[0].App != "b" {
		t.Fatalf("orden/filtro mal: %+v %v", all, err)
	}
	died, _ := db.ListEvents(10, "died", "")
	if len(died) != 2 {
		t.Fatalf("filtro kind: %d", len(died))
	}
	one, _ := db.ListEvents(10, "", "a")
	if len(one) != 2 {
		t.Fatalf("filtro app: %d", len(one))
	}
	none, _ := db.ListEvents(1, "died", "")
	if len(none) != 1 {
		t.Fatalf("límite: %d", len(none))
	}
}

// TestSaveInfraSubeVersion: la infra es config viva; sin bump el hot-reload
// nunca vería cambios de globales/settings.
func TestSaveInfraSubeVersion(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v1 := db.ConfigVersion()
	if err := db.SaveInfra([]byte(`{"listen_http":":80"}`)); err != nil {
		t.Fatal(err)
	}
	v2 := db.ConfigVersion()
	if v2 <= v1 {
		t.Fatalf("config_version no subió: %d -> %d", v1, v2)
	}
	raw, err := db.LoadInfra()
	if err != nil || string(raw) != `{"listen_http":":80"}` {
		t.Fatalf("infra = %q %v", raw, err)
	}
}
