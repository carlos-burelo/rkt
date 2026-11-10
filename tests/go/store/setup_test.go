package store

import (
	"testing"
	"time"
)

func TestSetupStateOneTime(t *testing.T) {
	db, err := Open(t.TempDir() + "/rkt.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	st, err := db.NewSetupState(time.Minute)
	if err != nil || st == "" {
		t.Fatalf("crear state: %v", err)
	}
	if db.ConsumeSetupState("otro") {
		t.Fatal("state ajeno no debe valer")
	}
	if !db.ConsumeSetupState(st) {
		t.Fatal("state válido debe consumirse")
	}
	if db.ConsumeSetupState(st) {
		t.Fatal("state no debe reutilizarse")
	}
}

func TestSetupStateExpira(t *testing.T) {
	db, err := Open(t.TempDir() + "/rkt.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	st, err := db.NewSetupState(-time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if db.ConsumeSetupState(st) {
		t.Fatal("state caducado no debe valer")
	}
}
