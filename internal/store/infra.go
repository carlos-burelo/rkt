// Config de infraestructura persistida en SQLite (única fuente).
// Apps/rutas viven en sus tablas; aquí solo lo que antes era YAML.
package store

import (
	"database/sql"
	"errors"
	"time"
)

// SaveInfra guarda el JSON de infra (id=1, upsert).
func (d *DB) SaveInfra(data []byte) error {
	if d == nil || d.sql == nil {
		return errors.New("db cerrada")
	}
	return saveInfra(d.sql, data)
}

func saveInfra(q queryer, data []byte) error {
	if _, err := q.Exec(
		`INSERT INTO infra (id, data, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET data=excluded.data, updated_at=excluded.updated_at`,
		string(data), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	// La infra (globales, settings) también es config viva: que watchMerged
	// la recargue, no solo los cambios de flota.
	return bumpVersion(q)
}

// LoadInfra devuelve el JSON de infra (nil si nunca se guardó).
func (d *DB) LoadInfra() ([]byte, error) { return loadInfra(d.sql) }

func loadInfra(q queryer) ([]byte, error) {
	var data string
	err := q.QueryRow(`SELECT data FROM infra WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(data), nil
}
