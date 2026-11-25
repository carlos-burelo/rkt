package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"rkt/internal/authz"
	"rkt/internal/pg"
)

// backupDir dir de dumps custom (-Fc) por esquema.
func (d *Deps) backupDir() string {
	return filepath.Join(filepath.Dir(d.LogsDir), "data", "backups")
}

// handlePgIndexes GET lista (?app=&table=) o POST crea o DELETE borra.
func (d *Deps) handlePgIndexes(w http.ResponseWriter, r *http.Request) {
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
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		idx, err := pg.ListIndexes(ctx, db, rec.Schema, table)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if idx == nil {
			idx = []pg.IndexInfo{}
		}
		writeJSON(w, map[string]any{"app": app, "table": table, "indexes": idx})
	case http.MethodPost:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		var in struct {
			App     string   `json:"app"`
			Table   string   `json:"table"`
			Name    string   `json:"name"`
			Columns []string `json:"columns"`
			Unique  bool     `json:"unique"`
			Method  string   `json:"method"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildCreateIndex("s", nonEmptyTable(in.Table), in.Name, in.Columns, in.Unique, in.Method); err != nil {
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
		if err := pg.CreateIndexExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), strings.TrimSpace(in.Name), in.Columns, in.Unique, in.Method); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "created": true})
	case http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		q := r.URL.Query()
		app, name := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("index"))
		db, rec, err := d.pgAppDB(app)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.DropIndexExec(ctx, db, rec.Schema, name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "index": name, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST|DELETE")
	}
}

// handlePgConstraints GET restricciones (?app=&table=).
func (d *Deps) handlePgConstraints(w http.ResponseWriter, r *http.Request) {
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
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cons, err := pg.ListConstraints(ctx, db, rec.Schema, table)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if cons == nil {
		cons = []pg.ConstraintInfo{}
	}
	writeJSON(w, map[string]any{"app": app, "table": table, "constraints": cons})
}

// handlePgPolicy POST crea o DELETE borra policies RLS.
func (d *Deps) handlePgPolicy(w http.ResponseWriter, r *http.Request) {
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var in struct {
			App   string   `json:"app"`
			Table string   `json:"table"`
			Name  string   `json:"name"`
			Cmd   string   `json:"cmd"`
			Roles []string `json:"roles"`
			Using string   `json:"using"`
			Check string   `json:"check"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildCreatePolicy("s", nonEmptyTable(in.Table), in.Name, in.Cmd, in.Roles, in.Using, in.Check); err != nil {
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
		if err := pg.CreatePolicyExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Name, in.Cmd, in.Roles, in.Using, in.Check); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "policy": in.Name, "created": true})
	case http.MethodDelete:
		q := r.URL.Query()
		app, table, name := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("table")), strings.TrimSpace(q.Get("name"))
		db, rec, err := d.pgAppDB(app)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.DropPolicyExec(ctx, db, rec.Schema, table, name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "table": table, "policy": name, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST|DELETE")
	}
}

// handlePgRename POST {app, table, new_table}.
func (d *Deps) handlePgRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App      string `json:"app"`
		Table    string `json:"table"`
		NewTable string `json:"new_table"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if _, err := pg.BuildRenameTable("s", nonEmptyTable(in.Table), strings.TrimSpace(in.NewTable)); err != nil {
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
	if err := pg.RenameTableExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), strings.TrimSpace(in.NewTable)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.NewTable, "renamed": true})
}

// handlePgColumnRename POST {app, table, column, new_column}.
func (d *Deps) handlePgColumnRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App       string `json:"app"`
		Table     string `json:"table"`
		Column    string `json:"column"`
		NewColumn string `json:"new_column"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if _, err := pg.BuildRenameColumn("s", nonEmptyTable(in.Table), in.Column, in.NewColumn); err != nil {
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
	if err := pg.RenameColumnExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), strings.TrimSpace(in.Column), strings.TrimSpace(in.NewColumn)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "column": in.NewColumn, "renamed": true})
}

// handlePgColumnAlter POST {app, table, column, type?, default?, drop_default?, not_null?}.
// type/default son punteros: ausente = sin cambio. not_null igual.
func (d *Deps) handlePgColumnAlter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App         string  `json:"app"`
		Table       string  `json:"table"`
		Column      string  `json:"column"`
		Type        *string `json:"type"`
		Default     *string `json:"default"`
		DropDefault bool    `json:"drop_default"`
		NotNull     *bool   `json:"not_null"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if in.Type == nil && in.Default == nil && !in.DropDefault && in.NotNull == nil {
		writeErr(w, http.StatusBadRequest, "sin cambios (type, default/drop_default o not_null)")
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
	table := strings.TrimSpace(in.Table)
	col := strings.TrimSpace(in.Column)
	if in.Type != nil {
		if err := pg.AlterColumnTypeExec(ctx, db, rec.Schema, table, col, *in.Type); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if in.DropDefault {
		if err := pg.AlterColumnDefaultExec(ctx, db, rec.Schema, table, col, "", true); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if in.Default != nil {
		if err := pg.AlterColumnDefaultExec(ctx, db, rec.Schema, table, col, *in.Default, false); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if in.NotNull != nil {
		if err := pg.AlterColumnNullableExec(ctx, db, rec.Schema, table, col, *in.NotNull); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, map[string]any{"app": in.App, "table": table, "column": col, "altered": true})
}

// handlePgTruncate POST {app, table}.
func (d *Deps) handlePgTruncate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App   string `json:"app"`
		Table string `json:"table"`
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
	if err := pg.TruncateExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "truncated": true})
}

// handlePgDuplicate POST {app, table, new_table, with_data?}.
func (d *Deps) handlePgDuplicate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App      string `json:"app"`
		Table    string `json:"table"`
		NewTable string `json:"new_table"`
		WithData bool   `json:"with_data"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if _, err := pg.BuildDuplicate("s", nonEmptyTable(in.Table), strings.TrimSpace(in.NewTable), in.WithData); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := pg.DuplicateExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), strings.TrimSpace(in.NewTable), in.WithData); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.NewTable, "duplicated": true})
}

// handlePgBackups GET lista (?app=) o DELETE borra (?app=&name=).
func (d *Deps) handlePgBackups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !requireOp(w, r, authz.OpPgRead) {
			return
		}
		app := strings.TrimSpace(r.URL.Query().Get("app"))
		list, err := pg.ListBackups(d.backupDir(), app)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "backups": list})
	case http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		q := r.URL.Query()
		app, name := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("name"))
		if err := pg.DeleteBackup(d.backupDir(), app, name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "name": name, "deleted": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|DELETE")
	}
}

// handlePgBackupDownload GET descarga (?app=&name=) como attachment.
func (d *Deps) handlePgBackupDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, name := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("name"))
	p, err := pg.BackupPath(d.backupDir(), app, name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filepath.Base(p)))
	http.ServeFile(w, r, p)
}

// csvEsc celda CSV (comillas y NULL vacíos, espejo del frontend).
func csvEsc(v any) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprint(v)
	if strings.ContainsAny(s, "\",\n\r") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// handlePgRowsExport GET CSV (?app=&table=&format=csv + filtros del visor).
func (d *Deps) handlePgRowsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, table := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("table"))
	filters, err := pg.ParseFilters(q["filter"])
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rq := pg.RowsQuery{
		OrderCol: strings.TrimSpace(q.Get("order")), OrderDir: strings.TrimSpace(q.Get("dir")),
		Filters: filters, Search: strings.TrimSpace(q.Get("search")),
	}
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	searchCols := []string{}
	if strings.TrimSpace(rq.Search) != "" {
		if cis, cerr := pg.ColumnTypes(ctx, db, rec.Schema, table); cerr == nil {
			for _, c := range cis {
				t := strings.ToLower(c.Type)
				if strings.Contains(t, "char") || t == "text" || t == "uuid" || t == "json" || t == "jsonb" || t == "inet" {
					searchCols = append(searchCols, c.Name)
				}
			}
		}
	}
	sel, args, err := pg.ExportQuery(rec.Schema, table, rq, searchCols)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rs, err := db.QueryContext(ctx, sel, args...)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer rs.Close()
	var sb strings.Builder
	cols, err := rs.Columns()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	esc := func(v any) string { return csvEsc(v) }
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = esc(c)
	}
	sb.WriteString(strings.Join(head, ",") + "\n")
	n := 0
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = esc(string(b))
			} else {
				row[i] = esc(v)
			}
		}
		sb.WriteString(strings.Join(row, ",") + "\n")
		n++
	}
	if err := rs.Err(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, table+".csv"))
	w.Header().Set("X-Rows", fmt.Sprint(n))
	_, _ = w.Write([]byte(sb.String()))
}

// handlePgRowsImport POST {app, table, columns, rows} lote 1..1000.
func (d *Deps) handlePgRowsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App     string   `json:"app"`
		Table   string   `json:"table"`
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	dec.UseNumber()
	if err := dec.Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	// json.Number → int64/float64/string (igual que CLI mustJSONMap).
	norm := make([][]any, 0, len(in.Rows))
	for _, row := range in.Rows {
		nr := make([]any, 0, len(row))
		for _, v := range row {
			if n, ok := v.(json.Number); ok {
				if i, err := n.Int64(); err == nil {
					nr = append(nr, i)
				} else if f, err := n.Float64(); err == nil {
					nr = append(nr, f)
				} else {
					nr = append(nr, n.String())
				}
				continue
			}
			nr = append(nr, v)
		}
		norm = append(norm, nr)
	}
	if _, _, err := pg.BuildInsertMany("s", nonEmptyTable(in.Table), in.Columns, norm); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	n, err := pg.InsertManyExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Columns, norm)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "inserted": n})
}

// handlePgQueryBatch POST {app, sql} multi-sentencia (máx 20).
func (d *Deps) handlePgQueryBatch(w http.ResponseWriter, r *http.Request) {
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
	if len(pg.SplitStatements(in.SQL)) > 20 {
		writeErr(w, http.StatusBadRequest, "máx 20 sentencias por lote")
		return
	}
	db, _, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	res, err := pg.QueryMulti(ctx, db, in.SQL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if res == nil {
		res = []pg.QueryResult{}
	}
	writeJSON(w, map[string]any{"app": in.App, "results": res, "count": len(res)})
}

// handlePgQueryExplain POST {app, sql} plan EXPLAIN.
func (d *Deps) handlePgQueryExplain(w http.ResponseWriter, r *http.Request) {
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
	rows, cols, isJSON, err := pg.Explain(ctx, db, in.SQL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "columns": cols, "rows": rows, "json": isJSON})
}
