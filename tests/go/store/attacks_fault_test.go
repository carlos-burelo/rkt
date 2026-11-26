package store

import (
	"testing"
	"time"
)

// Con la DB cerrada cada operación nueva devuelve error en vez de pánico.
func TestAttacksYGitCredsConDBCerrada(t *testing.T) {
	db := newDB(t)
	_ = db.Close()
	now := time.Now()
	if err := db.AddAttacks([]Attack{{Ts: now.Format(time.RFC3339), IP: "1.1.1.1"}}); err == nil {
		t.Error("AddAttacks")
	}
	if _, err := db.PruneAttacks(now); err == nil {
		t.Error("PruneAttacks")
	}
	if _, err := db.SummarizeAttacks(24, 5, now); err == nil {
		t.Error("SummarizeAttacks")
	}
	if _, err := db.ListGitCreds(); err == nil {
		t.Error("ListGitCreds")
	}
	if err := db.AddGitCred(GitCred{Name: "x", Kind: "pat"}); err == nil {
		t.Error("AddGitCred")
	}
	if err := db.DeleteGitCred("x"); err == nil {
		t.Error("DeleteGitCred")
	}
}

// Valores por defecto de la ventana y del top.
func TestSummarizeAttacksDefaults(t *testing.T) {
	db := newDB(t)
	sum, err := db.SummarizeAttacks(0, 0, time.Now())
	if err != nil || sum.Hours != 24 || sum.Total != 0 || len(sum.Countries) != 0 {
		t.Fatalf("defaults: %+v %v", sum, err)
	}
}
