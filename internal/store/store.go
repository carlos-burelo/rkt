// Package store: persistencia SQLite (rkt.db junto a la config).
// Solo historial consultable: deployments + events. La config YAML
// sigue siendo la fuente de verdad (legible, difeable en git).
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// v5: instalaciones las manda la API (ya no se cachean en DB).
	`DROP TABLE IF EXISTS installations;`,
	// v6: auth local (usuarios + sesiones bearer).
	`CREATE TABLE IF NOT EXISTS users (
		name TEXT PRIMARY KEY,
		pass_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		created_at TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS sessions (
		token_hash TEXT PRIMARY KEY,
		user TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT '',
		expires_at TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user);`,
	// v7: trazabilidad deploy (quién + con qué env).
	`ALTER TABLE deployments ADD COLUMN triggered_by TEXT NOT NULL DEFAULT '';
	ALTER TABLE deployments ADD COLUMN env_snapshot TEXT NOT NULL DEFAULT '{}';`,
	// v8: reglas de alerta (motor las evalúa en tick).
	`CREATE TABLE IF NOT EXISTS alert_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		app TEXT NOT NULL DEFAULT '*',
		kind TEXT NOT NULL DEFAULT 'failed',
		threshold REAL NOT NULL DEFAULT 1,
		window_min INTEGER NOT NULL DEFAULT 60,
		last_fired TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT ''
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
	// busy_timeout ANTES de WAL: journal_mode necesita lock y, sin timeout,
	// dos clientes (MCP + servicio) fallaban con SQLITE_BUSY al abrir.
	for _, p := range []string{
		"PRAGMA busy_timeout=10000",
		"PRAGMA journal_mode=WAL",
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

// Close cierra (nil-safe: no-op).
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

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
	ID          int64  `json:"id"`
	App         string `json:"app"`
	Rev         string `json:"rev"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at"`
	FinishedAt  string `json:"finished_at"`
	TriggeredBy string `json:"triggered_by"`
	EnvSnapshot string `json:"env_snapshot"`
}

// InsertDeployment abre fila 'running', devuelve id. Guarda quién
// dispara y snapshot del env (trazabilidad; rollback los hereda).
func (d *DB) InsertDeployment(app, rev, logPath, triggeredBy string, env map[string]string) (int64, error) {
	snap := "{}"
	if env != nil {
		if b, err := json.Marshal(env); err == nil {
			snap = string(b)
		}
	}
	res, err := d.sql.Exec(
		`INSERT INTO deployments(app, rev, status, started_at, log_path, triggered_by, env_snapshot) VALUES(?,?,?,?,?,?,?)`,
		app, rev, "running", time.Now().UTC().Format(time.RFC3339), logPath, triggeredBy, snap)
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
	q := `SELECT id, app, rev, status, started_at, finished_at, triggered_by, env_snapshot FROM deployments ORDER BY id DESC LIMIT ?`
	var rows *sql.Rows
	var err error
	if app == "" {
		rows, err = d.sql.Query(q, n)
	} else {
		rows, err = d.sql.Query(
			`SELECT id, app, rev, status, started_at, finished_at, triggered_by, env_snapshot FROM deployments WHERE app=? ORDER BY id DESC LIMIT ?`,
			app, n)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Deployment{}
	for rows.Next() {
		var dep Deployment
		if err := rows.Scan(&dep.ID, &dep.App, &dep.Rev, &dep.Status, &dep.StartedAt, &dep.FinishedAt, &dep.TriggeredBy, &dep.EnvSnapshot); err != nil {
			return nil, err
		}
		out = append(out, dep)
	}
	return out, rows.Err()
}

// DeploymentLogPath devuelve (app, log_path) de un deploy para tail.
func (d *DB) DeploymentLogPath(id int64) (string, string, bool) {
	var app, lp string
	if err := d.sql.QueryRow(`SELECT app, log_path FROM deployments WHERE id=?`, id).Scan(&app, &lp); err != nil {
		return "", "", false
	}
	if lp == "" {
		return "", "", false
	}
	return app, lp, true
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

// Event suceso auditable.
type Event struct {
	ID   int64  `json:"id"`
	Ts   string `json:"ts"`
	Kind string `json:"kind"`
	App  string `json:"app"`
	Msg  string `json:"msg"`
}

// ListEvents últimos N (kind/app "" = todos).
func (d *DB) ListEvents(n int, kind, app string) ([]Event, error) {
	q := `SELECT id, ts, kind, app, msg FROM events`
	var conds []string
	var args []any
	if kind != "" {
		conds = append(conds, "kind=?")
		args = append(args, kind)
	}
	if app != "" {
		conds = append(conds, "app=?")
		args = append(args, app)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, n)
	rows, err := d.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Ts, &e.Kind, &e.App, &e.Msg); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
