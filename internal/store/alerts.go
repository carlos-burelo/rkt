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
	Enabled   bool    `json:"enabled"`
	ChannelID int64   `json:"channel_id"`
	LastFired string  `json:"last_fired"`
	CreatedAt string  `json:"created_at"`
}

// AlertChannel destino de notificación.
type AlertChannel struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// AlertFire disparo histórico de una regla.
type AlertFire struct {
	ID         int64   `json:"id"`
	RuleID     int64   `json:"rule_id"`
	Ts         string  `json:"ts"`
	Value      float64 `json:"value"`
	Threshold  float64 `json:"threshold"`
	Message    string  `json:"message"`
	ResolvedAt string  `json:"resolved_at"`
}

// AlertDelivery entrega pendiente o completada.
type AlertDelivery struct {
	ID        int64  `json:"id"`
	FireID    int64  `json:"fire_id"`
	ChannelID int64  `json:"channel_id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	NextAt    string `json:"next_at"`
	LastError string `json:"last_error"`
	Message   string `json:"message,omitempty"`
}

// ValidAlertKind kinds aceptados por el evaluador.
func ValidAlertKind(kind string) bool {
	switch kind {
	case "failed", "error_rate", "latency_p95", "memory_mb", "down_min", "disk_pct", "cpu_pct", "mem_pct", "temp_c", "cert_days", "deploy_failed":
		return true
	}
	return false
}

// ListRules todas.
func (d *DB) ListRules() ([]AlertRule, error) {
	rows, err := d.sql.Query(`SELECT id, app, kind, threshold, window_min, enabled, channel_id, last_fired, created_at FROM alert_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertRule{}
	for rows.Next() {
		var r AlertRule
		if err := rows.Scan(&r.ID, &r.App, &r.Kind, &r.Threshold, &r.WindowMin, &r.Enabled, &r.ChannelID, &r.LastFired, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddRule valida y crea con valores por defecto (compatibilidad CLI/MCP).
func (d *DB) AddRule(app, kind string, threshold float64, windowMin int) (int64, error) {
	return d.AddRuleFull(app, kind, threshold, windowMin, true, 0)
}

// AddRuleFull valida y crea una regla completa.
func (d *DB) AddRuleFull(app, kind string, threshold float64, windowMin int, enabled bool, channelID int64) (int64, error) {
	if !ValidAlertKind(kind) {
		return 0, fmt.Errorf("kind inválido (failed|error_rate|latency_p95|memory_mb|down_min|disk_pct|cpu_pct|mem_pct|temp_c|cert_days|deploy_failed)")
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
		`INSERT INTO alert_rules(app, kind, threshold, window_min, enabled, channel_id, created_at) VALUES(?,?,?,?,?,?,?)`,
		app, kind, threshold, windowMin, enabled, channelID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, d.bumpVersion()
}

// UpdateRule cambia todos los campos editables.
func (d *DB) UpdateRule(id int64, app, kind string, threshold float64, windowMin int, enabled bool, channelID int64) error {
	if !ValidAlertKind(kind) {
		return fmt.Errorf("kind inválido")
	}
	if threshold <= 0 || windowMin < 5 || windowMin > 1440 {
		return fmt.Errorf("threshold > 0 y window_min 5..1440")
	}
	if app == "" {
		app = "*"
	}
	res, err := d.sql.Exec(`UPDATE alert_rules SET app=?, kind=?, threshold=?, window_min=?, enabled=?, channel_id=? WHERE id=?`,
		app, kind, threshold, windowMin, enabled, channelID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("regla #%d inexistente", id)
	}
	return d.bumpVersion()
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

// ListChannels canales ordenados.
func (d *DB) ListChannels() ([]AlertChannel, error) {
	rows, err := d.sql.Query(`SELECT id, name, url, enabled, created_at FROM alert_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertChannel{}
	for rows.Next() {
		var c AlertChannel
		if err := rows.Scan(&c.ID, &c.Name, &c.URL, &c.Enabled, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddChannel crea un canal webhook.
func (d *DB) AddChannel(name, url string, enabled bool) (int64, error) {
	res, err := d.sql.Exec(`INSERT INTO alert_channels(name, url, enabled, created_at) VALUES(?,?,?,?)`,
		name, url, enabled, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, d.bumpVersion()
}

// UpdateChannel edita un canal.
func (d *DB) UpdateChannel(id int64, name, url string, enabled bool) error {
	res, err := d.sql.Exec(`UPDATE alert_channels SET name=?, url=?, enabled=? WHERE id=?`, name, url, enabled, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("canal #%d inexistente", id)
	}
	return d.bumpVersion()
}

// DeleteChannel borra un canal y desasigna reglas.
func (d *DB) DeleteChannel(id int64) error {
	if _, err := d.sql.Exec(`UPDATE alert_rules SET channel_id=0 WHERE channel_id=?`, id); err != nil {
		return err
	}
	res, err := d.sql.Exec(`DELETE FROM alert_channels WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("canal #%d inexistente", id)
	}
	return d.bumpVersion()
}

// AddFire guarda un disparo.
func (d *DB) AddFire(ruleID int64, value, threshold float64, message string) (int64, error) {
	res, err := d.sql.Exec(`INSERT INTO alert_fires(rule_id, ts, value, threshold, message) VALUES(?,?,?,?,?)`,
		ruleID, time.Now().UTC().Format(time.RFC3339), value, threshold, message)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListFires historial de una regla.
func (d *DB) ListFires(ruleID int64, limit int) ([]AlertFire, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.sql.Query(`SELECT id, rule_id, ts, value, threshold, message, resolved_at FROM alert_fires WHERE rule_id=? ORDER BY id DESC LIMIT ?`, ruleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertFire{}
	for rows.Next() {
		var f AlertFire
		if err := rows.Scan(&f.ID, &f.RuleID, &f.Ts, &f.Value, &f.Threshold, &f.Message, &f.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// EnqueueDelivery crea una entrega pendiente por canal.
func (d *DB) EnqueueDelivery(fireID, channelID int64) (int64, error) {
	res, err := d.sql.Exec(`INSERT INTO alert_deliveries(fire_id, channel_id, status, attempts, next_at) VALUES(?,?, 'pending', 0, ?)`,
		fireID, channelID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PendingDeliveries entrega pendientes con URL del canal.
func (d *DB) PendingDeliveries(limit int) ([]AlertDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := d.sql.Query(`SELECT d.id, d.fire_id, d.channel_id, c.url, d.status, d.attempts, d.next_at, d.last_error, f.message
		FROM alert_deliveries d
		JOIN alert_channels c ON c.id=d.channel_id
		LEFT JOIN alert_fires f ON f.id=d.fire_id
		WHERE d.status='pending' AND c.enabled=1 ORDER BY d.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertDelivery{}
	for rows.Next() {
		var x AlertDelivery
		if err := rows.Scan(&x.ID, &x.FireID, &x.ChannelID, &x.URL, &x.Status, &x.Attempts, &x.NextAt, &x.LastError, &x.Message); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// MarkDelivery actualiza estado de entrega.
func (d *DB) MarkDelivery(id int64, status string, attempts int, nextAt, lastErr string) error {
	_, err := d.sql.Exec(`UPDATE alert_deliveries SET status=?, attempts=?, next_at=?, last_error=? WHERE id=?`,
		status, attempts, nextAt, lastErr, id)
	return err
}
