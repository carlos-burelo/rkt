package store

// Bordes de validación y no-encontrado del store: cada rama que el gate
// de cobertura (90%) echaba en falta. Sin inyección de fallos: solo
// entradas inválidas, ids inexistentes y límites fuera de rango.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAlertRuleEdges(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.AddRuleFull("app", "failed", 1, 60, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		kind      string
		threshold float64
		window    int
		app       string
		id        int64
		wantErr   bool
	}{
		{"kind inválido", "nope", 1, 60, "app", id, true},
		{"threshold cero", "failed", 0, 60, "app", id, true},
		{"window corta", "failed", 1, 1, "app", id, true},
		{"window larga", "failed", 1, 2000, "app", id, true},
		{"app vacía vale", "failed", 1, 60, "", id, false},
		{"id inexistente", "failed", 1, 60, "app", 999999, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := db.UpdateRule(tc.id, tc.app, tc.kind, tc.threshold, tc.window, true, 0)
			if (err != nil) != tc.wantErr {
				t.Fatalf("UpdateRule err=%v, quiere error=%v", err, tc.wantErr)
			}
		})
	}
	if err := db.UpdateChannel(999999, "x", "https://example.com/x", true); err == nil {
		t.Fatal("UpdateChannel inexistente debe fallar")
	}
	if err := db.DeleteChannel(999999); err == nil {
		t.Fatal("DeleteChannel inexistente debe fallar")
	}
	if _, err := db.ListFires(id, 0); err != nil {
		t.Fatalf("ListFires sin límite: %v", err)
	}
	if _, err := db.ListFires(id, 999); err != nil {
		t.Fatalf("ListFires límite alto: %v", err)
	}
	if _, err := db.PendingDeliveries(0); err != nil {
		t.Fatalf("PendingDeliveries sin límite: %v", err)
	}
	if _, err := db.PendingDeliveries(999); err != nil {
		t.Fatalf("PendingDeliveries límite alto: %v", err)
	}
}

func TestUserEdges(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateUser("a1", "password-larga-1", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole("nadie", "nope"); err == nil {
		t.Fatal("rol inválido debe fallar")
	}
	if err := db.SetUserRole("nadie", RoleViewer); err == nil {
		t.Fatal("usuario inexistente debe fallar")
	}
	if err := db.SetUserRole("a1", RoleViewer); err == nil {
		t.Fatal("degradar el último admin debe fallar")
	}
	if err := db.SetUserDisabled("nadie", true); err == nil {
		t.Fatal("usuario inexistente debe fallar")
	}
	if err := db.SetUserDisabled("a1", true); err == nil {
		t.Fatal("deshabilitar el último admin debe fallar")
	}
	if err := db.RevokeSession("a1", "noexiste"); err == nil {
		t.Fatal("sesión inexistente debe fallar")
	}
	for _, u := range []UserDump{
		{Name: "", PassHash: "h", Role: RoleViewer},
		{Name: "x", PassHash: "h", Role: "otro"},
		{Name: "x", PassHash: "", Role: RoleViewer},
	} {
		if err := db.ImportUsers([]UserDump{u}); err == nil {
			t.Fatalf("import inválido debe fallar: %+v", u)
		}
	}
	if _, err := db.ListAudit(0); err != nil {
		t.Fatalf("ListAudit sin límite: %v", err)
	}
	if _, err := db.ListAudit(999); err != nil {
		t.Fatalf("ListAudit límite alto: %v", err)
	}
}

func TestCronLimitEdges(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.InsertCronRun("a", "job", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ListCronRuns("a", "job", 0); err != nil {
		t.Fatalf("n=0: %v", err)
	}
	if _, err := db.ListCronRuns("a", "job", 999); err != nil {
		t.Fatalf("n alto: %v", err)
	}
}

func TestSetupEdges(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.ConsumeSetupState("") {
		t.Fatal("state vacío no debe valer")
	}
}

func TestBackupEdges(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bloqueo := filepath.Join(dir, "fichero")
	if err := os.WriteFile(bloqueo, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.Backup(filepath.Join(bloqueo, "copia.db")); err == nil {
		t.Fatal("Backup con dir bloqueado debe fallar")
	}
}
