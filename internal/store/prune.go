// Poda de historial viejo (deployments/events). No toca config viva.
package store

import "time"

// PruneDeployments borra salidas terminadas anteriores a cutoff.
func (d *DB) PruneDeployments(before time.Time) (int64, error) {
	res, err := d.sql.Exec(
		`DELETE FROM deployments WHERE finished_at != '' AND finished_at < ?`,
		before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneEvents borra eventos anteriores a cutoff.
func (d *DB) PruneEvents(before time.Time) (int64, error) {
	res, err := d.sql.Exec(`DELETE FROM events WHERE ts < ?`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
