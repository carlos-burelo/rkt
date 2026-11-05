package store

import (
	"database/sql"
	"errors"
	"time"
)

// CronRun ejecución de una tarea programada.
type CronRun struct {
	ID         int64  `json:"id"`
	App        string `json:"app"`
	Job        string `json:"job"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	LogPath    string `json:"log_path"`
	Message    string `json:"message"`
}

// InsertCronRun abre fila running.
func (d *DB) InsertCronRun(app, job, logPath string) (int64, error) {
	res, err := d.sql.Exec(`INSERT INTO cron_runs(app, job, started_at, status, log_path)
		VALUES(?,?,?, 'running', ?)`,
		app, job, time.Now().UTC().Format(time.RFC3339), logPath)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishCronRun cierra con estado, exit code y mensaje.
func (d *DB) FinishCronRun(id int64, status string, exitCode int, message string) error {
	_, err := d.sql.Exec(`UPDATE cron_runs SET finished_at=?, status=?, exit_code=?, message=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339), status, exitCode, message, id)
	return err
}

// ListCronRuns últimas N; app/job vacíos = todos.
func (d *DB) ListCronRuns(app, job string, n int) ([]CronRun, error) {
	if n <= 0 || n > 500 {
		n = 50
	}
	q := `SELECT id, app, job, started_at, finished_at, status, exit_code, log_path, message
		FROM cron_runs`
	args := []any{}
	where := []string{}
	if app != "" {
		where = append(where, "app=?")
		args = append(args, app)
	}
	if job != "" {
		where = append(where, "job=?")
		args = append(args, job)
	}
	if len(where) > 0 {
		q += " WHERE " + where[0]
		for _, w := range where[1:] {
			q += " AND " + w
		}
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, n)
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CronRun{}
	for rows.Next() {
		var r CronRun
		if err := rows.Scan(&r.ID, &r.App, &r.Job, &r.StartedAt, &r.FinishedAt, &r.Status, &r.ExitCode, &r.LogPath, &r.Message); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetCronRunLog fija la ruta del log de una ejecución.
func (d *DB) SetCronRunLog(id int64, path string) error {
	_, err := d.sql.Exec(`UPDATE cron_runs SET log_path=? WHERE id=?`, path, id)
	return err
}

// CronRunByID devuelve una ejecución por id.
func (d *DB) CronRunByID(id int64) (CronRun, bool, error) {
	var r CronRun
	err := d.sql.QueryRow(`SELECT id, app, job, started_at, finished_at, status, exit_code, log_path, message
		FROM cron_runs WHERE id=?`, id).
		Scan(&r.ID, &r.App, &r.Job, &r.StartedAt, &r.FinishedAt, &r.Status, &r.ExitCode, &r.LogPath, &r.Message)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CronRun{}, false, nil
		}
		return CronRun{}, false, err
	}
	return r, true, nil
}

// LastCronRun última ejecución de una tarea.
func (d *DB) LastCronRun(app, job string) (CronRun, bool, error) {
	runs, err := d.ListCronRuns(app, job, 1)
	if err != nil || len(runs) == 0 {
		return CronRun{}, false, err
	}
	return runs[0], true, nil
}
