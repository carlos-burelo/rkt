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

// nonEmptyTable evita que la pre-validación offline pase "" como tabla.
func nonEmptyTable(t string) string {
	if strings.TrimSpace(t) == "" {
		return "t"
	}
	return strings.TrimSpace(t)
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

// handlePgTables lista tablas del esquema (?app=) con conteo por tabla.
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
	counts, err := pg.TableCounts(ctx, db, rec.Schema, tables)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type detail struct {
		Name string `json:"name"`
		Rows int64  `json:"rows"`
	}
	det := make([]detail, 0, len(tables))
	for _, t := range tables {
		det = append(det, detail{Name: t, Rows: counts[t]})
	}
	writeJSON(w, map[string]any{"app": app, "tables": tables, "detail": det})
}

// handlePgRows visor Supabase: GET pagina con filtros/orden/búsqueda;
// POST inserta, PATCH actualiza por PK, DELETE borra por PK o filtros.
func (d *Deps) handlePgRows(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.pgRowsList(w, r)
	case http.MethodPost, http.MethodPatch, http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		switch r.Method {
		case http.MethodPost:
			d.pgRowInsert(w, r)
		case http.MethodPatch:
			d.pgRowUpdate(w, r)
		default:
			d.pgRowDelete(w, r)
		}
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST|PATCH|DELETE")
	}
}

func (d *Deps) pgRowsList(w http.ResponseWriter, r *http.Request) {
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, table := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("table"))
	n := queryInt(r, "n", 100)
	if n > 500 {
		n = 500
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	filters, err := pg.ParseFilters(q["filter"])
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	order, dir := strings.TrimSpace(q.Get("order")), strings.TrimSpace(q.Get("dir"))
	search := strings.TrimSpace(q.Get("search"))
	if len(search) > 200 {
		writeErr(w, http.StatusBadRequest, "search máx 200 caracteres")
		return
	}
	// Validación barata antes de conectar (testeable sin Postgres).
	probeTable := table
	if probeTable == "" {
		probeTable = "t"
	}
	if _, _, err := pg.BuildSelect("s", probeTable, pg.RowsQuery{
		Limit: n, Offset: (page - 1) * n,
		OrderCol: order, OrderDir: dir, Filters: filters, Search: search,
	}, []string{"s"}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cols, rows, total, err := pg.TableRowsEx(ctx, db, rec.Schema, table, pg.RowsQuery{
		Limit: n, Offset: (page - 1) * n,
		OrderCol: order, OrderDir: dir, Filters: filters, Search: search,
	})
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
	writeJSON(w, map[string]any{
		"app": app, "table": table, "columns": cols, "types": typed,
		"rows": rows, "total": total, "page": page,
		"order": order, "dir": dir, "filters": filters, "search": search,
	})
}

func (d *Deps) pgRowInsert(w http.ResponseWriter, r *http.Request) {
	var in struct {
		App   string         `json:"app"`
		Table string         `json:"table"`
		Data  map[string]any `json:"data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	table := strings.TrimSpace(in.Table)
	if table == "" {
		table = "t"
	}
	if _, _, err := pg.BuildInsert("s", table, in.Data); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	row, err := pg.InsertRow(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "row": row})
}

func (d *Deps) pgRowUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		App   string         `json:"app"`
		Table string         `json:"table"`
		PK    map[string]any `json:"pk"`
		Patch map[string]any `json:"patch"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	table := strings.TrimSpace(in.Table)
	if table == "" {
		table = "t"
	}
	if _, _, err := pg.BuildUpdate("s", table, in.PK, in.Patch); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	n, err := pg.UpdateRow(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.PK, in.Patch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "updated": n})
}

func (d *Deps) pgRowDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		App     string           `json:"app"`
		Table   string           `json:"table"`
		PK      map[string]any   `json:"pk,omitempty"`
		PKs     []map[string]any `json:"pks,omitempty"`
		Filters []string         `json:"filters,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	table := strings.TrimSpace(in.Table)
	if table == "" {
		table = "t"
	}
	filters, err := pg.ParseFilters(in.Filters)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Pre-valida una de las ramas (la otra se valida en el loop/exec).
	if len(in.PKs) > 0 {
		if len(in.PKs) > 100 {
			writeErr(w, http.StatusBadRequest, "máx 100 filas por borrado")
			return
		}
		if len(in.Filters) > 0 || len(in.PK) > 0 {
			writeErr(w, http.StatusBadRequest, "pks o pk o filters, no combinados")
			return
		}
		if _, _, err := pg.BuildDelete("s", table, in.PKs[0], nil); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if _, _, err := pg.BuildDelete("s", table, in.PK, filters); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// Borrado múltiple por lista de PKs (selección del grid).
	if len(in.PKs) > 0 {
		var total int64
		for _, pk := range in.PKs {
			n, err := pg.DeleteRows(ctx, db, rec.Schema, strings.TrimSpace(in.Table), pk, nil)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			total += n
		}
		writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "deleted": total})
		return
	}
	n, err := pg.DeleteRows(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.PK, filters)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "deleted": n})
}

// handlePgSchema inspector y DDL: GET describe (?app=&table= o ?app= para
// todas), POST crea tabla, DELETE borra tabla.
func (d *Deps) handlePgSchema(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if table != "" {
			s, err := pg.DescribeTable(ctx, db, rec.Schema, table)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			pols, _ := pg.ListPolicies(ctx, db, rec.Schema, table)
			if pols == nil {
				pols = []pg.Policy{}
			}
			writeJSON(w, map[string]any{"app": app, "schema": s, "policies": pols})
			return
		}
		tables, err := pg.Tables(ctx, db, rec.Schema)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]pg.TableSchema, 0, len(tables))
		for _, t := range tables {
			s, err := pg.DescribeTable(ctx, db, rec.Schema, t)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			out = append(out, s)
		}
		writeJSON(w, map[string]any{"app": app, "tables": out})
	case http.MethodPost:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		var in struct {
			App     string         `json:"app"`
			Table   string         `json:"table"`
			Columns []pg.ColumnDef `json:"columns"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildCreateTable("s", nonEmptyTable(in.Table), in.Columns); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.CreateTableExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Columns); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "created": true})
	case http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
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
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.DropTableExec(ctx, db, rec.Schema, table); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "table": table, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST|DELETE")
	}
}

// handlePgSchemaColumn añade (POST) o quita (DELETE) columnas.
func (d *Deps) handlePgSchemaColumn(w http.ResponseWriter, r *http.Request) {
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var in struct {
			App    string       `json:"app"`
			Table  string       `json:"table"`
			Column pg.ColumnDef `json:"column"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildAddColumn("s", nonEmptyTable(in.Table), in.Column); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.AddColumnExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Column); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "column": in.Column.Name, "added": true})
	case http.MethodDelete:
		q := r.URL.Query()
		app, table, col := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("table")), strings.TrimSpace(q.Get("column"))
		db, rec, err := d.pgAppDB(app)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.DropColumnExec(ctx, db, rec.Schema, table, col); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "table": table, "column": col, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST|DELETE")
	}
}

// handlePgSchemaRLS activa/desactiva RLS en una tabla.
func (d *Deps) handlePgSchemaRLS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App    string `json:"app"`
		Table  string `json:"table"`
		Enable bool   `json:"enable"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := pg.SetRLS(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Enable); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "rls": in.Enable})
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
