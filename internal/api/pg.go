package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"rkt/internal/authz"
	"rkt/internal/cfgfile"
	"rkt/internal/config"
	"rkt/internal/pg"
	"rkt/internal/store"
)

// pgStore adapta store.DB al Registry mínimo de pg.
type pgStore struct{ db *store.DB }

func (s pgStore) PgGet(app string) (pg.Record, bool, error) {
	p, ok, err := s.db.PgGet(app)
	if err != nil || !ok {
		return pg.Record{}, ok, err
	}
	return pg.Record{App: p.App, Schema: p.Schema, Role: p.Role, Mode: p.Mode, DB: p.DB}, true, nil
}

func (s pgStore) PgList() ([]pg.Record, error) {
	all, err := s.db.PgList()
	if err != nil {
		return nil, err
	}
	out := make([]pg.Record, 0, len(all))
	for _, p := range all {
		out = append(out, pg.Record{App: p.App, Schema: p.Schema, Role: p.Role, Mode: p.Mode, DB: p.DB})
	}
	return out, nil
}

func (s pgStore) PgPut(app, schema, role, mode, db string) error {
	return s.db.PgProvision(app, schema, role, mode, db)
}

func (s pgStore) PgRemove(app string) error {
	return s.db.PgRemove(app)
}

// pgConfig lee la config del Postgres gestionado desde settings.
func pgConfig(db *store.DB) (pg.Config, error) {
	return pg.LoadConfig(db.GetSetting)
}

// handlePg lista bases aprovisionadas (GET) o aprovisiona una app (POST).
func (d *Deps) handlePg(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	if r.Method == http.MethodPost {
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
	}
	cfg, err := pgConfig(d.DB)
	if err != nil {
		writeJSON(w, map[string]any{"enabled": false, "apps": []any{}})
		return
	}
	if r.Method == http.MethodGet {
		recs, err := pgStore{d.DB}.PgList()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		apps := make([]map[string]any, 0, len(recs))
		for _, p := range recs {
			apps = append(apps, map[string]any{
				"app": p.App, "schema": p.Schema, "role": p.Role, "mode": p.Mode,
				"url": pg.ConnectionURL(cfg.Host, cfg.Port, p.DB, p.Role),
			})
		}
		writeJSON(w, map[string]any{
			"enabled": true, "host": cfg.Host, "port": cfg.Port, "database": cfg.DB, "apps": apps,
		})
		return
	}
	var in struct {
		App  string `json:"app"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if !config.ValidAppName(in.App) {
		writeErr(w, http.StatusBadRequest, "app inválida [a-z0-9-]")
		return
	}
	// La base cuelga de una app existente (DATABASE_URL va a su env):
	// validar antes de tocar Postgres evita huérfanos (esquema sin app).
	if cfgfile.FindApp(d.Live.Get(), in.App) == nil {
		writeErr(w, http.StatusBadRequest, "app "+in.App+" inexistente: créala primero (apps add)")
		return
	}
	url, err := pg.ProvisionApp(r.Context(), pgStore{d.DB}, cfg, in.App, in.Mode)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := cfgfile.SetEnv(d.DB, in.App, "DATABASE_URL", url); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	rec, _, _ := pgStore{d.DB}.PgGet(in.App)
	writeJSON(w, map[string]any{
		"app": in.App, "schema": rec.Schema, "role": rec.Role, "mode": rec.Mode, "url": url,
	})
}

// pgAppDB conecta como rol de la app (trust loopback, sin password).
func (d *Deps) pgAppDB(app string) (*sql.DB, pg.Record, error) {
	var zero pg.Record
	rec, ok, err := pgStore{d.DB}.PgGet(app)
	if err != nil {
		return nil, zero, err
	}
	if !ok {
		return nil, zero, fmt.Errorf("pg: %q sin base", app)
	}
	cfg, err := pg.LoadConfig(d.DB.GetSetting)
	if err != nil {
		return nil, zero, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return nil, zero, err
	}
	return db, rec, nil
}

// handlePgTables lista tablas del esquema (?app=).
func (d *Deps) handlePgTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	app := strings.TrimSpace(r.URL.Query().Get("app"))
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tables, err := pg.Tables(ctx, db, rec.Schema)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tables == nil {
		tables = []string{}
	}
	writeJSON(w, map[string]any{"app": app, "tables": tables})
}

// handlePgRows pagina una tabla (?app=&table=&n=&page=). Lectura: viewer.
func (d *Deps) handlePgRows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, table := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("table"))
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	n := queryInt(r, "n", 100)
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cols, rows, total, err := pg.TableRows(ctx, db, rec.Schema, table, n, (page-1)*n)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	type colJSON struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	byName := map[string]string{}
	if cis, cerr := pg.ColumnTypes(ctx, db, rec.Schema, table); cerr == nil {
		for _, c := range cis {
			byName[c.Name] = c.Type
		}
	}
	typed := make([]colJSON, len(cols))
	for i, c := range cols {
		typed[i] = colJSON{Name: c, Type: byName[c]}
	}
	if cols == nil {
		cols = []string{}
	}
	if rows == nil {
		rows = [][]any{}
	}
	writeJSON(w, map[string]any{"app": app, "table": table, "columns": cols, "types": typed, "rows": rows, "total": total, "page": page})
}

// handlePgQuery consola SQL (POST {app, sql}). Todo con confirmación en UI.
func (d *Deps) handlePgQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App string `json:"app"`
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	db, _, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := pg.Query(ctx, db, in.SQL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, res)
}

// handlePgRestore vuelca un backup custom sobre la app (POST {app, file}).
func (d *Deps) handlePgRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App  string `json:"app"`
		File string `json:"file"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	reg := pgStore{d.DB}
	if _, ok, err := reg.PgGet(strings.TrimSpace(in.App)); err != nil || !ok {
		writeErr(w, http.StatusBadRequest, "pg: app sin base")
		return
	}
	cfg, err := pg.LoadConfig(d.DB.GetSetting)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	if err := pg.Restore(ctx, cfg.Bins.PgRestore, cfg.Host, cfg.Port, cfg.User, cfg.Pass, cfg.DB, in.File); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "restored": in.File})
}
func (d *Deps) handlePgSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/pg/"), "/")
	parts := strings.Split(rest, "/")
	name := parts[0]
	if !config.ValidAppName(name) {
		writeErr(w, http.StatusBadRequest, "app inválida [a-z0-9-]")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	reg := pgStore{d.DB}
	switch {
	case r.Method == http.MethodDelete && len(parts) == 1:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		cfg, err := pgConfig(d.DB)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := pg.DropApp(r.Context(), reg, cfg, name); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": name, "dropped": true})
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "backup":
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		cfg, err := pgConfig(d.DB)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		outDir := filepath.Join(filepath.Dir(d.LogsDir), "data", "backups")
		path, err := pg.BackupApp(r.Context(), cfg, reg, name, outDir)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": name, "path": path})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "DELETE|POST backup")
	}
}
