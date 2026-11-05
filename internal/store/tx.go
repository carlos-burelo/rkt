package store

import (
	"context"
	"database/sql"

	"rkt-proxy/internal/config"
)

// Tx vista de una transacción activa. Todas las lecturas y escrituras de un
// read-modify-write deben pasar por aquí: nunca por *DB (otra conexión).
type Tx struct {
	ctx  context.Context
	conn *sql.Conn
}

func (t *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return t.conn.ExecContext(t.ctx, query, args...)
}

func (t *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.conn.QueryContext(t.ctx, query, args...)
}

func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	return t.conn.QueryRowContext(t.ctx, query, args...)
}

// LoadFleet lee la flota dentro de la transacción.
func (t *Tx) LoadFleet() (*FleetData, error) { return loadFleet(t) }

// ImportFleet reemplaza la flota dentro de la transacción.
func (t *Tx) ImportFleet(cfg *config.Config) error { return importFleet(t, cfg) }

// LoadInfra lee el JSON de infraestructura dentro de la transacción.
func (t *Tx) LoadInfra() ([]byte, error) { return loadInfra(t) }

// SaveInfra guarda el JSON de infraestructura dentro de la transacción.
func (t *Tx) SaveInfra(data []byte) error { return saveInfra(t, data) }

// GetSetting lee un setting dentro de la transacción.
func (t *Tx) GetSetting(key string) string {
	v, _ := getSetting(t, key)
	return v
}

// SetSetting escribe un setting dentro de la transacción.
func (t *Tx) SetSetting(key, value string) error { return setSetting(t, key, value) }
