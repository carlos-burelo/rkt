package cli

import (
	"path/filepath"
	"testing"

	"rkt/internal/logx"
	"rkt/internal/store"
)

// auditLocal deja rastro de las mutaciones del CLI (la API audita por
// middleware y MCP por gate; la vía directa a SQLite no pasaba por ninguno).
func TestAuditLocal(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.db")
	auditLocal(cfg, "users add", "ana")
	db, err := store.Open(logx.DBPath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.ListAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != "cli users add" || rows[0].Target != "ana" || rows[0].Actor == "" {
		t.Fatalf("auditoría: %+v", rows)
	}
}
