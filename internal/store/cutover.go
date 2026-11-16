package store

import "time"

// Cutover journal de un corte deploy/rollback: intent (started) antes de
// persistir el corte, confirmación (done) tras adoptar el canario.
// El arranque marca aborted lo pendiente: DB y runtime pueden haber
// quedado en desacuerdo si el servicio murió a mitad del swap.
type Cutover struct {
	ID         int64  `json:"id"`
	App        string `json:"app"`
	FromDir    string `json:"from_dir"`
	ToDir      string `json:"to_dir"`
	Port       string `json:"port"`
	Rev        string `json:"rev"`
	Actor      string `json:"actor"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// InsertCutover abre un intent 'started', devuelve id.
func (d *DB) InsertCutover(app, fromDir, toDir, port, rev, actor string) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO cutovers(app, from_dir, to_dir, port, rev, actor, status, started_at) VALUES(?,?,?,?,?,?,'started',?)`,
		app, fromDir, toDir, port, rev, actor, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishCutover cierra el intent (done/aborted).
func (d *DB) FinishCutover(id int64, status string) error {
	_, err := d.sql.Exec(
		`UPDATE cutovers SET status=?, finished_at=? WHERE id=?`,
		status, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// PendingCutovers intents sin confirmar (el servicio murió a mitad).
func (d *DB) PendingCutovers() ([]Cutover, error) {
	rows, err := d.sql.Query(
		`SELECT id, app, from_dir, to_dir, port, rev, actor, status, started_at, finished_at FROM cutovers WHERE status='started' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Cutover
	for rows.Next() {
		var c Cutover
		if err := rows.Scan(&c.ID, &c.App, &c.FromDir, &c.ToDir, &c.Port, &c.Rev, &c.Actor, &c.Status, &c.StartedAt, &c.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
