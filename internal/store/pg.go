package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PgApp mapea una app a su esquema en el Postgres gestionado.
type PgApp struct {
	App       string `json:"app"`
	Schema    string `json:"schema"`
	Role      string `json:"role"`
	Mode      string `json:"mode"` // dedicated|shared
	DB        string `json:"db"`
	CreatedAt string `json:"created_at"`
}

// PgProvision registra el esquema de una app (una por app).
func (d *DB) PgProvision(app, schema, role, mode, db string) error {
	if app == "" || schema == "" || role == "" || db == "" {
		return fmt.Errorf("pg: app/schema/role/db obligatorios")
	}
	if mode != "dedicated" && mode != "shared" {
		return fmt.Errorf("pg: mode inválido %q (dedicated|shared)", mode)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return d.WithTx(context.Background(), func(tx *Tx) error {
		var exists string
		err := tx.QueryRow(`SELECT app FROM pg_apps WHERE app=?`, app).Scan(&exists)
		switch {
		case err == nil:
			return fmt.Errorf("pg: %q ya tiene esquema", app)
		case errors.Is(err, sql.ErrNoRows):
			// libre: sigue al insert
		default:
			return err
		}
		if _, err := tx.Exec(`INSERT INTO pg_apps(app, schema_name, role, mode, db, created_at) VALUES(?,?,?,?,?,?)`,
			app, schema, role, mode, db, now); err != nil {
			return fmt.Errorf("pg provision: %w", err)
		}
		return bumpVersion(tx)
	})
}

// PgGet devuelve el mapeo de una app ("", false si no tiene).
func (d *DB) PgGet(app string) (PgApp, bool, error) {
	var p PgApp
	err := d.sql.QueryRow(`SELECT app, schema_name, role, mode, db, created_at FROM pg_apps WHERE app=?`, app).
		Scan(&p.App, &p.Schema, &p.Role, &p.Mode, &p.DB, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PgApp{}, false, nil
		}
		return PgApp{}, false, err
	}
	return p, true, nil
}

// PgList lista todos los mapeos ordenados por app.
func (d *DB) PgList() ([]PgApp, error) {
	rows, err := d.sql.Query(`SELECT app, schema_name, role, mode, db, created_at FROM pg_apps ORDER BY app`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PgApp{}
	for rows.Next() {
		var p PgApp
		if err := rows.Scan(&p.App, &p.Schema, &p.Role, &p.Mode, &p.DB, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PgRemove borra el mapeo (el esquema/role se borran en Postgres aparte).
func (d *DB) PgRemove(app string) error {
	return d.WithTx(context.Background(), func(tx *Tx) error {
		res, err := tx.Exec(`DELETE FROM pg_apps WHERE app=?`, app)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("pg: %q sin esquema", app)
		}
		return bumpVersion(tx)
	})
}
