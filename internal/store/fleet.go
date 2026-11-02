// Flota en DB: apps, routes y settings. La config YAML conserva la
// infra estable (listeners, ACME...); cada campo tiene un solo hogar.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"rkt-proxy/internal/config"
)

// execer lo cumplen *sql.DB y *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// GetSetting lee un setting ("").
func (d *DB) GetSetting(key string) string {
	var v string
	_ = d.sql.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	return v
}

// SetSetting escribe setting (upsert) + bump versión.
func (d *DB) SetSetting(key, value string) error {
	if _, err := d.sql.Exec(`INSERT INTO settings(key, value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
		return err
	}
	return d.bumpVersion()
}

// ConfigVersion versión de flota (meta). 0 = DB sin inicializar.
func (d *DB) ConfigVersion() int64 {
	var v int64
	_ = d.sql.QueryRow(`SELECT value FROM meta WHERE key='config_version'`).Scan(&v)
	return v
}

// bumpVersion +1 versión de flota.
func (d *DB) bumpVersion() error {
	_, err := d.sql.Exec(`INSERT INTO meta(key, value) VALUES('config_version','1')
		ON CONFLICT(key) DO UPDATE SET value=CAST(CAST(value AS INTEGER)+1 AS TEXT)`)
	return err
}

// UpsertApp crea/actualiza app + bump.
func (d *DB) UpsertApp(a *config.App) error {
	if err := insertApp(d.sql, a); err != nil {
		return err
	}
	return d.bumpVersion()
}

// DeleteApp borra por nombre + bump. True si existía.
func (d *DB) DeleteApp(name string) (bool, error) {
	res, err := d.sql.Exec(`DELETE FROM apps WHERE name=?`, name)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	return true, d.bumpVersion()
}

// AddRoute agrega ruta + bump. Devuelve id.
func (d *DB) AddRoute(hosts []string, target string) (int64, error) {
	h, _ := json.Marshal(hosts)
	res, err := d.sql.Exec(`INSERT INTO routes(hosts, target) VALUES(?,?)`, string(h), target)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := d.bumpVersion(); err != nil {
		return 0, err
	}
	return id, nil
}

// RemoveHost quita un host de sus rutas (borra rutas vacías) + bump.
// Devuelve true si cambió algo.
func (d *DB) RemoveHost(host string) (bool, error) {
	rows, err := d.sql.Query(`SELECT id, hosts FROM routes`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	type row struct {
		id    int64
		hosts []string
	}
	var all []row
	for rows.Next() {
		var r row
		var h string
		if err := rows.Scan(&r.id, &h); err != nil {
			return false, err
		}
		if err := json.Unmarshal([]byte(h), &r.hosts); err != nil {
			return false, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	changed := false
	tx, err := d.sql.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	for _, r := range all {
		kept := []string{}
		for _, h := range r.hosts {
			if equalFold(h, host) {
				changed = true
				continue
			}
			kept = append(kept, h)
		}
		if len(kept) == len(r.hosts) {
			continue
		}
		if len(kept) == 0 {
			if _, err := tx.Exec(`DELETE FROM routes WHERE id=?`, r.id); err != nil {
				return false, err
			}
			continue
		}
		h, _ := json.Marshal(kept)
		if _, err := tx.Exec(`UPDATE routes SET hosts=? WHERE id=?`, string(h), r.id); err != nil {
			return false, err
		}
	}
	if !changed {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, d.bumpVersion()
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// SetAppEnv escribe una env var de una app + bump.
func (d *DB) SetAppEnv(name, key, value string) error {
	a, err := d.getApp(name)
	if err != nil {
		return err
	}
	if a.Env == nil {
		a.Env = map[string]string{}
	}
	a.Env[key] = value
	if err := insertApp(d.sql, a); err != nil {
		return err
	}
	return d.bumpVersion()
}

// UnsetAppEnv borra una env var + bump.
func (d *DB) UnsetAppEnv(name, key string) error {
	a, err := d.getApp(name)
	if err != nil {
		return err
	}
	delete(a.Env, key)
	if err := insertApp(d.sql, a); err != nil {
		return err
	}
	return d.bumpVersion()
}

func (d *DB) getApp(name string) (*config.App, error) {
	fleet, err := d.LoadFleet()
	if err != nil {
		return nil, err
	}
	for i := range fleet.Apps {
		if fleet.Apps[i].Name == name {
			return &fleet.Apps[i], nil
		}
	}
	return nil, fmt.Errorf("app %q inexistente", name)
}

func insertApp(ex execer, a *config.App) error {
	args, _ := json.Marshal(a.Args)
	env, _ := json.Marshal(a.Env)
	build := []string{}
	ref, repo, tokenEnv := "", "", ""
	auto := 0
	if a.Deploy != nil {
		repo, ref, tokenEnv = a.Deploy.Repo, a.Deploy.Ref, a.Deploy.TokenEnv
		if a.Deploy.Build != nil {
			build = a.Deploy.Build
		}
		if a.Deploy.Auto {
			auto = 1
		}
	}
	buildJSON, _ := json.Marshal(build)
	_, err := ex.Exec(`INSERT INTO apps(name, command, args, dir, env_file, env,
		restart_delay_ms, min_uptime_ms, memory_mb,
		deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_auto)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET command=excluded.command, args=excluded.args,
		dir=excluded.dir, env_file=excluded.env_file, env=excluded.env,
		restart_delay_ms=excluded.restart_delay_ms, min_uptime_ms=excluded.min_uptime_ms,
		memory_mb=excluded.memory_mb, deploy_repo=excluded.deploy_repo,
		deploy_ref=excluded.deploy_ref, deploy_token_env=excluded.deploy_token_env,
		deploy_build=excluded.deploy_build, deploy_auto=excluded.deploy_auto`,
		a.Name, a.Command, string(args), a.Dir, a.EnvFile, string(env),
		a.Delay().Milliseconds(), a.MinUp().Milliseconds(), a.MemoryMB,
		repo, ref, tokenEnv, string(buildJSON), auto)
	return err
}

// FleetData flota cruda para snapshot.
type FleetData struct {
	Apps   []config.App
	Routes []config.Route
}

func (d *DB) LoadFleet() (*FleetData, error) {
	out := &FleetData{}
	rows, err := d.sql.Query(`SELECT name, command, args, dir, env_file, env,
		restart_delay_ms, min_uptime_ms, memory_mb,
		deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_auto FROM apps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a config.App
		var argsS, envS, buildS string
		var delayMs, minUpMs, memMB, auto int64
		var repo, ref, tokenEnv string
		if err := rows.Scan(&a.Name, &a.Command, &argsS, &a.Dir, &a.EnvFile, &envS,
			&delayMs, &minUpMs, &memMB, &repo, &ref, &tokenEnv, &buildS, &auto); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(argsS), &a.Args)
		a.Env = map[string]string{}
		_ = json.Unmarshal([]byte(envS), &a.Env)
		a.SetDelays(time.Duration(delayMs)*time.Millisecond, time.Duration(minUpMs)*time.Millisecond)
		a.MemoryMB = uint64(memMB)
		if repo != "" {
			var build []string
			_ = json.Unmarshal([]byte(buildS), &build)
			a.Deploy = &config.Deploy{
				Repo: repo, Ref: ref, TokenEnv: tokenEnv,
				Build: build, Auto: auto != 0,
			}
		}
		out.Apps = append(out.Apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rrows, err := d.sql.Query(`SELECT hosts, target FROM routes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var r config.Route
		var h string
		if err := rrows.Scan(&h, &r.Target); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(h), &r.Hosts)
		out.Routes = append(out.Routes, r)
	}
	return out, rrows.Err()
}

func msToDur(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	if ms%1000 == 0 {
		return string(rune(0)) // placeholder jamás usado
	}
	return "0s"
}

// ImportFleet vuelca apps/routes de un YAML (bootstrap o import).
// Reemplaza flota. No toca deployments/events.
func (d *DB) ImportFleet(cfg *config.Config) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM apps`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM routes`); err != nil {
		return err
	}
	for i := range cfg.Apps {
		if err := insertApp(tx, &cfg.Apps[i]); err != nil {
			return fmt.Errorf("apps[%s]: %w", cfg.Apps[i].Name, err)
		}
	}
	for i := range cfg.Routes {
		hosts, _ := json.Marshal(cfg.Routes[i].Hosts)
		if _, err := tx.Exec(`INSERT INTO routes(hosts, target) VALUES(?,?)`,
			string(hosts), cfg.Routes[i].Target); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('config_version','1')
		ON CONFLICT(key) DO UPDATE SET value=CAST(CAST(value AS INTEGER)+1 AS TEXT)`); err != nil {
		return err
	}
	return tx.Commit()
}
