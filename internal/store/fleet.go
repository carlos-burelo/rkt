// Flota en DB: apps, routes y settings. La infra estable (listeners,
// ACME...) vive en la tabla infra; cada campo tiene un solo hogar.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/settings"
)

// execer lo cumplen *sql.DB, *sql.Tx y *Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// queryer añade lectura (flota, infra, settings).
type queryer interface {
	execer
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Settings conocidos de flota (hogar canónico: internal/settings).
const (
	KeyNodeBin    = settings.KeyNodeBin
	KeyPythonBin  = settings.KeyPythonBin
	KeyBaseDomain = settings.KeyBaseDomain
	KeyPortPool   = settings.KeyPortPool
	KeySiteName   = settings.KeySiteName
	KeyQuotaReqs  = settings.KeyQuotaReqs
	KeyQuotaBytes = settings.KeyQuotaBytes
	KeyPublicIP   = settings.KeyPublicIP

	KeyRetentionDeployDays = settings.KeyRetentionDeployDays
	KeyRetentionEventDays  = settings.KeyRetentionEventDays
	KeyRetentionLogDays    = settings.KeyRetentionLogDays
	KeyRetentionBackupDays = settings.KeyRetentionBackupDays
	KeyDashboardHost       = settings.KeyDashboardHost
	KeyDashboardListen     = settings.KeyDashboardListen
	KeyListenHTTP          = settings.KeyListenHTTP
	KeyListenHTTPS         = settings.KeyListenHTTPS
)

// KnownSettings lista para `settings list` y export.
func KnownSettings() []string { return settings.Keys() }

func getSetting(q queryer, key string) (string, error) {
	var v string
	err := q.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// GetSetting lee un setting ("").
func (d *DB) GetSetting(key string) string {
	v, _ := getSetting(d.sql, key)
	return v
}

func setSetting(q queryer, key, value string) error {
	if _, err := q.Exec(`INSERT INTO settings(key, value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
		return err
	}
	return bumpVersion(q)
}

// SetSetting escribe setting (upsert) + bump versión.
func (d *DB) SetSetting(key, value string) error { return setSetting(d.sql, key, value) }

// ConfigVersion versión de flota (meta). 0 = DB sin inicializar.
func (d *DB) ConfigVersion() int64 {
	var v int64
	_ = d.sql.QueryRow(`SELECT value FROM meta WHERE key='config_version'`).Scan(&v)
	return v
}

// bumpVersion +1 versión de flota.
func (d *DB) bumpVersion() error { return bumpVersion(d.sql) }

func bumpVersion(q execer) error {
	_, err := q.Exec(`INSERT INTO meta(key, value) VALUES('config_version','1')
		ON CONFLICT(key) DO UPDATE SET value=CAST(CAST(value AS INTEGER)+1 AS TEXT)`)
	return err
}

func insertApp(ex execer, a *config.App) error {
	args, _ := json.Marshal(a.Args)
	env, _ := json.Marshal(a.Env)
	build := []string{}
	release := []string{}
	ref, repo, tokenEnv := "", "", ""
	auto := 0
	if a.Deploy != nil {
		repo, ref, tokenEnv = a.Deploy.Repo, a.Deploy.Ref, a.Deploy.TokenEnv
		if a.Deploy.Build != nil {
			build = a.Deploy.Build
		}
		if a.Deploy.Release != nil {
			release = a.Deploy.Release
		}
		if a.Deploy.Auto {
			auto = 1
		}
	}
	buildJSON, _ := json.Marshal(build)
	releaseJSON, _ := json.Marshal(release)
	cronJSON, _ := json.Marshal(a.Cron)
	persistJSON, _ := json.Marshal(a.Persist)
	allowJSON, _ := json.Marshal(a.AllowIPs)
	maint := 0
	if a.Maintenance {
		maint = 1
	}
	disabled := 0
	if a.Disabled {
		disabled = 1
	}
	_, err := ex.Exec(`INSERT INTO apps(name, runtime, command, args, dir, env_file, env,
		restart_delay_ms, min_uptime_ms, memory_mb, disabled, health_path, static, cron, persist,
		basic_auth, allow_ips, maintenance,
		deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_release, deploy_auto)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET runtime=excluded.runtime,
		command=excluded.command, args=excluded.args,
		dir=excluded.dir, env_file=excluded.env_file, env=excluded.env,
		restart_delay_ms=excluded.restart_delay_ms, min_uptime_ms=excluded.min_uptime_ms,
		memory_mb=excluded.memory_mb, disabled=excluded.disabled,
		health_path=excluded.health_path, static=excluded.static, cron=excluded.cron,
		persist=excluded.persist, basic_auth=excluded.basic_auth,
		allow_ips=excluded.allow_ips, maintenance=excluded.maintenance,
		deploy_repo=excluded.deploy_repo,
		deploy_ref=excluded.deploy_ref, deploy_token_env=excluded.deploy_token_env,
		deploy_build=excluded.deploy_build, deploy_release=excluded.deploy_release,
		deploy_auto=excluded.deploy_auto`,
		a.Name, a.Runtime, a.Command, string(args), a.Dir, a.EnvFile, string(env),
		a.Delay().Milliseconds(), a.MinUp().Milliseconds(), a.MemoryMB,
		disabled, a.HealthPath, a.Static, string(cronJSON), string(persistJSON),
		a.BasicAuth, string(allowJSON), maint,
		repo, ref, tokenEnv, string(buildJSON), string(releaseJSON), auto)
	return err
}

// FleetData flota cruda para snapshot.
type FleetData struct {
	Apps   []config.App
	Routes []config.Route
}

func (d *DB) LoadFleet() (*FleetData, error) { return loadFleet(d.sql) }

func loadFleet(q queryer) (*FleetData, error) {
	out := &FleetData{}
	rows, err := q.Query(`SELECT name, runtime, command, args, dir, env_file, env,
		restart_delay_ms, min_uptime_ms, memory_mb, disabled, health_path, static, cron, persist,
		basic_auth, allow_ips, maintenance,
		deploy_repo, deploy_ref, deploy_token_env, deploy_build, deploy_release, deploy_auto FROM apps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a config.App
		var argsS, envS, buildS, releaseS, cronS, persistS, allowS string
		var delayMs, minUpMs, memMB, auto, disabled, maint int64
		var repo, ref, tokenEnv string
		if err := rows.Scan(&a.Name, &a.Runtime, &a.Command, &argsS, &a.Dir, &a.EnvFile, &envS,
			&delayMs, &minUpMs, &memMB, &disabled, &a.HealthPath, &a.Static, &cronS, &persistS,
			&a.BasicAuth, &allowS, &maint,
			&repo, &ref, &tokenEnv, &buildS, &releaseS, &auto); err != nil {
			return nil, err
		}
		a.Disabled = disabled != 0
		a.Maintenance = maint != 0
		_ = json.Unmarshal([]byte(cronS), &a.Cron)
		_ = json.Unmarshal([]byte(persistS), &a.Persist)
		_ = json.Unmarshal([]byte(allowS), &a.AllowIPs)
		if a.Runtime == "" {
			a.Runtime = config.RuntimeManaged
		}
		_ = json.Unmarshal([]byte(argsS), &a.Args)
		a.Env = map[string]string{}
		_ = json.Unmarshal([]byte(envS), &a.Env)
		a.SetDelays(time.Duration(delayMs)*time.Millisecond, time.Duration(minUpMs)*time.Millisecond)
		a.MemoryMB = uint64(memMB)
		if repo != "" {
			var build, release []string
			_ = json.Unmarshal([]byte(buildS), &build)
			_ = json.Unmarshal([]byte(releaseS), &release)
			a.Deploy = &config.Deploy{
				Repo: repo, Ref: ref, TokenEnv: tokenEnv,
				Build: build, Release: release, Auto: auto != 0,
			}
		}
		out.Apps = append(out.Apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rrows, err := q.Query(`SELECT hosts, target, path, port FROM routes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var r config.Route
		var h string
		if err := rrows.Scan(&h, &r.Target, &r.Path, &r.Port); err != nil {
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
	if err := importFleet(tx, cfg); err != nil {
		return err
	}
	return tx.Commit()
}

// importFleet reemplaza apps/rutas usando el executor dado (tx o *sql.Tx).
// No abre ni cierra transacción: el caller controla atomicidad.
func importFleet(ex execer, cfg *config.Config) error {
	if _, err := ex.Exec(`DELETE FROM apps`); err != nil {
		return err
	}
	if _, err := ex.Exec(`DELETE FROM routes`); err != nil {
		return err
	}
	for i := range cfg.Apps {
		if err := insertApp(ex, &cfg.Apps[i]); err != nil {
			return fmt.Errorf("apps[%s]: %w", cfg.Apps[i].Name, err)
		}
	}
	for i := range cfg.Routes {
		hosts, _ := json.Marshal(cfg.Routes[i].Hosts)
		if _, err := ex.Exec(`INSERT INTO routes(hosts, target, path, port) VALUES(?,?,?,?)`,
			string(hosts), cfg.Routes[i].Target, cfg.Routes[i].Path, cfg.Routes[i].Port); err != nil {
			return err
		}
	}
	return bumpVersion(ex)
}
