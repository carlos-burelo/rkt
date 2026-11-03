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

// Settings conocidos de flota (únicos literales canónicos).
const (
	KeyNodeBin    = "node_bin"
	KeyPythonBin  = "python_bin"
	KeyBaseDomain = "base_domain"
	KeyPortPool   = "port_pool"
	KeySiteName   = "site_name"
	KeyQuotaReqs  = "quota_reqs"
	KeyQuotaBytes = "quota_bytes"
)

// KnownSettings lista para `settings list` y export.
func KnownSettings() []string {
	return []string{KeyNodeBin, KeyPythonBin, KeyBaseDomain, KeyPortPool, KeySiteName, KeyQuotaReqs, KeyQuotaBytes}
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
	_, err := ex.Exec(`INSERT INTO apps(name, runtime, command, args, dir, env_file, env,
		restart_delay_ms, min_uptime_ms, memory_mb,
		deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_auto)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET runtime=excluded.runtime,
		command=excluded.command, args=excluded.args,
		dir=excluded.dir, env_file=excluded.env_file, env=excluded.env,
		restart_delay_ms=excluded.restart_delay_ms, min_uptime_ms=excluded.min_uptime_ms,
		memory_mb=excluded.memory_mb, deploy_repo=excluded.deploy_repo,
		deploy_ref=excluded.deploy_ref, deploy_token_env=excluded.deploy_token_env,
		deploy_build=excluded.deploy_build, deploy_auto=excluded.deploy_auto`,
		a.Name, a.Runtime, a.Command, string(args), a.Dir, a.EnvFile, string(env),
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
	rows, err := d.sql.Query(`SELECT name, runtime, command, args, dir, env_file, env,
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
		if err := rows.Scan(&a.Name, &a.Runtime, &a.Command, &argsS, &a.Dir, &a.EnvFile, &envS,
			&delayMs, &minUpMs, &memMB, &repo, &ref, &tokenEnv, &buildS, &auto); err != nil {
			return nil, err
		}
		if a.Runtime == "" {
			a.Runtime = config.RuntimeManaged
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
