// Package store: persistencia SQLite (rkt.db junto a la config).
// Solo historial consultable: deployments + events. La config YAML
// sigue siendo la fuente de verdad (legible, difeable en git).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// migrations en orden; user_version = índice aplicado.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS deployments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		app TEXT NOT NULL,
		rev TEXT NOT NULL,
		prev_rev TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT '',
		log_path TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_deployments_app ON deployments(app, id DESC);`,
	`CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT NOT NULL,
		kind TEXT NOT NULL,
		app TEXT NOT NULL DEFAULT '',
		msg TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts DESC);`,
	// v3: flota (apps/routes/settings/meta). YAML queda para infra estable.
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS apps (
		name TEXT PRIMARY KEY,
		command TEXT NOT NULL DEFAULT '',
		args TEXT NOT NULL DEFAULT '[]',
		dir TEXT NOT NULL DEFAULT '',
		env_file TEXT NOT NULL DEFAULT '',
		env TEXT NOT NULL DEFAULT '{}',
		restart_delay_ms INTEGER NOT NULL DEFAULT 5000,
		min_uptime_ms INTEGER NOT NULL DEFAULT 10000,
		memory_mb INTEGER NOT NULL DEFAULT 0,
		deploy_repo TEXT NOT NULL DEFAULT '',
		deploy_ref TEXT NOT NULL DEFAULT '',
		deploy_token_env TEXT NOT NULL DEFAULT '',
		deploy_build TEXT NOT NULL DEFAULT '[]',
		deploy_auto INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS routes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hosts TEXT NOT NULL,
		target TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);`,
	// v4: instalaciones GitHub App (browser linking, multi-org).
	`CREATE TABLE IF NOT EXISTS installations (
		id INTEGER PRIMARY KEY,
		account TEXT NOT NULL DEFAULT '',
		added_at TEXT NOT NULL DEFAULT ''
	);`,
}

// DB wrapper.
type DB struct {
	sql *sql.DB
}

// Open abre (creando dir) y migra. WAL + busy_timeout (locks Windows).
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	for _, p := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := sqlDB.Exec(p); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close cierra.
func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) migrate() error {
	var v int
	if err := d.sql.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		if _, err := d.sql.Exec(migrations[i]); err != nil {
			return fmt.Errorf("migración %d: %w", i+1, err)
		}
		if _, err := d.sql.Exec(fmt.Sprintf("PRAGMA user_version=%d", i+1)); err != nil {
			return err
		}
	}
	return nil
}

// Deployment fila de historial.
type Deployment struct {
	ID         int64  `json:"id"`
	App        string `json:"app"`
	Rev        string `json:"rev"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

// InsertDeployment abre fila 'running', devuelve id.
func (d *DB) InsertDeployment(app, rev, logPath string) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO deployments(app, rev, status, started_at, log_path) VALUES(?,?,?,?,?)`,
		app, rev, "running", time.Now().UTC().Format(time.RFC3339), logPath)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishDeployment cierra fila con estado final y rev publicado.
func (d *DB) FinishDeployment(id int64, status, prevRev, rev string) error {
	_, err := d.sql.Exec(
		`UPDATE deployments SET status=?, prev_rev=?, rev=?, finished_at=? WHERE id=?`,
		status, prevRev, rev, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// ListDeployments últimas N (app "" = todas).
func (d *DB) ListDeployments(app string, n int) ([]Deployment, error) {
	q := `SELECT id, app, rev, status, started_at, finished_at FROM deployments ORDER BY id DESC LIMIT ?`
	var rows *sql.Rows
	var err error
	if app == "" {
		rows, err = d.sql.Query(q, n)
	} else {
		rows, err = d.sql.Query(
			`SELECT id, app, rev, status, started_at, finished_at FROM deployments WHERE app=? ORDER BY id DESC LIMIT ?`,
			app, n)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deployment{}
	for rows.Next() {
		var dep Deployment
		if err := rows.Scan(&dep.ID, &dep.App, &dep.Rev, &dep.Status, &dep.StartedAt, &dep.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, dep)
	}
	return out, rows.Err()
}

// InsertEvent suceso auditable (died/restarted/failed/deploy...).
func (d *DB) InsertEvent(kind, app, msg string) error {
	_, err := d.sql.Exec(
		`INSERT INTO events(ts, kind, app, msg) VALUES(?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), kind, app, msg)
	return err
}

// CountEvents cuenta por kind (para tests/monitoreo).
func (d *DB) CountEvents(kind string) (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM events WHERE kind=?`, kind).Scan(&n)
	return n, err
}
