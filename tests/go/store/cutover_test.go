package store

import "testing"

// El journal registra el intent antes de persistir el corte y se confirma
// después de adoptar el canario. Un reinicio a mitad deja started.
func TestCutoverJournal(t *testing.T) {
	db := newDB(t)
	if p, err := db.PendingCutovers(); err != nil || len(p) != 0 {
		t.Fatalf("pendientes al inicio: %v %v", p, err)
	}
	id, err := db.InsertCutover("w", "apps/w.a", "apps/w.b", "3001", "abc123", "tester")
	if err != nil || id == 0 {
		t.Fatalf("insert: %d %v", id, err)
	}
	pend, err := db.PendingCutovers()
	if err != nil || len(pend) != 1 {
		t.Fatalf("pendientes: %v %v", pend, err)
	}
	p := pend[0]
	if p.App != "w" || p.ToDir != "apps/w.b" || p.Port != "3001" || p.Status != "started" {
		t.Fatalf("intent raro: %+v", p)
	}
	if err := db.FinishCutover(id, "done"); err != nil {
		t.Fatal(err)
	}
	if p, err := db.PendingCutovers(); err != nil || len(p) != 0 {
		t.Fatalf("pendientes tras done: %v %v", p, err)
	}
}
