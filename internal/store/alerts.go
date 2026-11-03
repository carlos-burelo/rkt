// Alertas: reglas evaluadas en tick (ver internal/alerts).
// kinds: failed (deploys/app caídas) | error_rate (5xx en ventana).
package store

import (
	"fmt"
	"time"
)

// AlertRule fila.
type AlertRule struct {
	ID        int64   `json:"id"`
	App       string  `json:"app"`
	Kind      string  `json:"kind"`
	Threshold float64 `json:"threshold"`
	WindowMin int     `json:"window_min"`
	LastFired string  `json:"last_fired"`
	CreatedAt string  `json:"created_at"`
}

// ListRules todas.
func (d *DB) ListRules() ([]AlertRule, error) {
	rows, err := d.sql.Query(`SELECT id, app, kind, threshold, window_min, last_fired, created_at FROM alert_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertRule{}
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.App, &r.Kind, &r.Threshold, &r.WindowMin, &r.LastFired, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddRule valida y crea (operator+ vía API).
func (d *DB) AddRule(app, kind string, threshold float64, windowMin int) (int64, error) {
	if kind != "failed" && kind != "error_rate" {
		return 0, fmt.Errorf("kind inválido (failed|error_rate)")
	}
	if threshold <= 0 {
		return 0, fmt.Errorf("threshold debe ser > 0")
	}
	if windowMin < 5 || windowMin > 1440 {
		return 0, fmt.Errorf("window_min 5..1440")
	}
	if app == "" {
		app = "*"
	}
	res, err := d.sql.Exec(
		`INSERT INTO alert_rules(app, kind, threshold, window_min, created_at) VALUES(?,?,?,?,?)`,
		app, kind, threshold, windowMin, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, d.bumpVersion()
}

// DeleteRule borra.
func (d *DB) DeleteRule(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM alert_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("regla #%d inexistente", id)
	}
	return d.bumpVersion()
}

// MarkFired registra último disparo (cooldown = ventana).
func (d *DB) MarkFired(id int64, at time.Time) error {
	_, err := d.sql.Exec(`UPDATE alert_rules SET last_fired=? WHERE id=?`,
		at.UTC().Format(time.RFC3339), id)
	return err
}
