package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"rkt-proxy/internal/config"
)

func TestWithTxCoversTxView(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "tx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	err = db.WithTx(context.Background(), func(tx *Tx) error {
		if _, err := tx.Exec(`INSERT INTO events(kind, app, msg, ts) VALUES('died','a','x','2026-01-01T00:00:00Z')`); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT COUNT(*) FROM events`)
		if err != nil {
			return err
		}
		rows.Close()
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n); err != nil || n != 1 {
			return errors.New("queryrow")
		}
		if _, err := tx.LoadFleet(); err != nil {
			return err
		}
		cfg := &config.Config{
			Apps: []config.App{{
				Name: "a", Command: "c", Dir: "/x",
				Env: map[string]string{"PORT": "3000"},
			}},
			Routes: []config.Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:3000"}},
		}
		if err := tx.ImportFleet(cfg); err != nil {
			return err
		}
		if _, err := tx.LoadInfra(); err != nil {
			return err
		}
		if err := tx.SaveInfra([]byte(`{"site":"rkt"}`)); err != nil {
			return err
		}
		if tx.GetSetting("nope") != "" {
			return errors.New("setting inesperado")
		}
		return tx.SetSetting("site_name", "rkt")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := db.GetSetting("site_name"); got != "rkt" {
		t.Fatalf("setting = %q", got)
	}
	if fleet, err := db.LoadFleet(); err != nil || len(fleet.Apps) != 1 || len(fleet.Routes) != 1 {
		t.Fatalf("fleet = %+v %v", fleet, err)
	}

	// un error de la función revierte la transacción
	err = db.WithTx(context.Background(), func(tx *Tx) error {
		if _, err := tx.Exec(`INSERT INTO events(kind, app, msg, ts) VALUES('died','b','y','2026-01-01T00:00:00Z')`); err != nil {
			return err
		}
		return errors.New("abortar")
	})
	if err == nil {
		t.Fatal("WithTx debía devolver el error")
	}
	if n, _ := db.CountEvents("died"); n != 1 {
		t.Fatalf("rollback incompleto: %d", n)
	}
}
