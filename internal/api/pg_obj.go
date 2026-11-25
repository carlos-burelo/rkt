package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"rkt/internal/authz"
	"rkt/internal/pg"
)

// handlePgViews GET lista (?app=) o detalle (?app=&name=) de views+matviews.
func (d *Deps) handlePgViews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, name := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("name"))
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if name != "" {
		v, err := pg.GetViewDef(ctx, db, rec.Schema, name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "view": v})
		return
	}
	views, err := pg.ListViews(ctx, db, rec.Schema)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mats, err := pg.ListMatviews(ctx, db, rec.Schema)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	all := append(views, mats...)
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	if all == nil {
		all = []pg.ViewInfo{}
	}
	writeJSON(w, map[string]any{"app": app, "views": all, "count": len(all)})
}

// handlePgView POST crea o DELETE borra (kind auto-detectado).
func (d *Deps) handlePgView(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		var in struct {
			App          string `json:"app"`
			Name         string `json:"name"`
			SQL          string `json:"sql"`
			Materialized bool   `json:"materialized"`
			OrReplace    bool   `json:"or_replace"`
			WithData     bool   `json:"with_data"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildCreateView("s", nonEmptyTable(in.Name), in.SQL, in.Materialized, in.OrReplace, in.WithData); err != nil {
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
		if err := pg.CreateViewExec(ctx, db, rec.Schema, strings.TrimSpace(in.Name), in.SQL, in.Materialized, in.OrReplace, in.WithData); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "view": in.Name, "created": true})
	case http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		q := r.URL.Query()
		app, name := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("name"))
		db, rec, err := d.pgAppDB(app)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		v, err := pg.GetViewDef(ctx, db, rec.Schema, name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := pg.DropViewExec(ctx, db, rec.Schema, name, v.Kind == "materialized"); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "view": name, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST|DELETE")
	}
}

// handlePgMatviewRefresh POST {app, name, with_data?}.
func (d *Deps) handlePgMatviewRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App      string `json:"app"`
		Name     string `json:"name"`
		WithData bool   `json:"with_data"`
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
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := pg.RefreshMatviewExec(ctx, db, rec.Schema, strings.TrimSpace(in.Name), in.WithData); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "view": in.Name, "refreshed": true})
}

// handlePgFunctions GET lista (?app=) o definición (?app=&func=).
func (d *Deps) handlePgFunctions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, fn := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("func"))
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if fn != "" {
		f, err := pg.GetRoutineDef(ctx, db, rec.Schema, fn)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		f.Kind = "function"
		writeJSON(w, map[string]any{"app": app, "function": f})
		return
	}
	list, err := pg.ListFunctions(ctx, db, rec.Schema)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if list == nil {
		list = []pg.FuncInfo{}
	}
	writeJSON(w, map[string]any{"app": app, "functions": list, "count": len(list)})
}

// handlePgFunction DELETE ?app=&func=identidad.
func (d *Deps) handlePgFunction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	q := r.URL.Query()
	app, fn := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("func"))
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := pg.DropRoutineExec(ctx, db, rec.Schema, fn, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": app, "function": fn, "dropped": true})
}

// handlePgProcedures GET lista (?app=) o definición (?app=&procedure=).
func (d *Deps) handlePgProcedures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpPgRead) {
		return
	}
	q := r.URL.Query()
	app, pr := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("procedure"))
	db, rec, err := d.pgAppDB(app)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if pr != "" {
		f, err := pg.GetRoutineDef(ctx, db, rec.Schema, pr)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		f.Kind = "procedure"
		writeJSON(w, map[string]any{"app": app, "procedure": f})
		return
	}
	list, err := pg.ListProcedures(ctx, db, rec.Schema)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if list == nil {
		list = []pg.FuncInfo{}
	}
	writeJSON(w, map[string]any{"app": app, "procedures": list, "count": len(list)})
}

// handlePgProcedure DELETE borra o POST /call ejecuta CALL.
func (d *Deps) handlePgProcedure(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		// POST directo = CALL {app, procedure: "nombre(args valores)"}.
		var in struct {
			App       string `json:"app"`
			Procedure string `json:"procedure"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildCallProcedure("s", in.Procedure); err != nil {
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
		res, err := pg.CallProcedureExec(ctx, db, rec.Schema, in.Procedure)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "procedure": in.Procedure, "result": res})
	case http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		q := r.URL.Query()
		app, pr := strings.TrimSpace(q.Get("app")), strings.TrimSpace(q.Get("procedure"))
		db, rec, err := d.pgAppDB(app)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := pg.DropRoutineExec(ctx, db, rec.Schema, pr, true); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "procedure": pr, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST|DELETE")
	}
}

// handlePgTriggers GET lista (?app=&table=).
func (d *Deps) handlePgTriggers(w http.ResponseWriter, r *http.Request) {
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
	list, err := pg.ListTriggers(ctx, db, rec.Schema, table)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": app, "table": table, "triggers": list, "count": len(list)})
}

// handlePgTrigger POST crea o DELETE borra.
func (d *Deps) handlePgTrigger(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
		var in struct {
			App         string   `json:"app"`
			Table       string   `json:"table"`
			Name        string   `json:"name"`
			Timing      string   `json:"timing"`
			Events      []string `json:"events"`
			Orientation string   `json:"orientation"`
			Function    string   `json:"function"`
			Condition   string   `json:"condition"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if _, err := pg.BuildCreateTrigger("s", nonEmptyTable(in.Table), in.Name, in.Timing, in.Events, in.Orientation, in.Function, in.Condition); err != nil {
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
		if err := pg.CreateTriggerExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Name, in.Timing, in.Events, in.Orientation, in.Function, in.Condition); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "trigger": in.Name, "created": true})
	case http.MethodDelete:
		if !requireOp(w, r, authz.OpPgWrite) {
			return
		}
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
		if err := pg.DropTriggerExec(ctx, db, rec.Schema, table, name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"app": app, "table": table, "trigger": name, "dropped": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "POST|DELETE")
	}
}

// handlePgTriggerToggle POST {app, table, name, enable}.
func (d *Deps) handlePgTriggerToggle(w http.ResponseWriter, r *http.Request) {
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
		Name   string `json:"name"`
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
	if err := pg.ToggleTriggerExec(ctx, db, rec.Schema, strings.TrimSpace(in.Table), in.Name, in.Enable); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "table": in.Table, "trigger": in.Name, "enabled": in.Enable})
}

// handlePgSequences GET lista (?app=).
func (d *Deps) handlePgSequences(w http.ResponseWriter, r *http.Request) {
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
	list, err := pg.ListSequences(ctx, db, rec.Schema)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": app, "sequences": list, "count": len(list)})
}

// handlePgSequenceRestart POST {app, name, with?}.
func (d *Deps) handlePgSequenceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App  string `json:"app"`
		Name string `json:"name"`
		With *int64 `json:"with"`
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
	if err := pg.RestartSequenceExec(ctx, db, rec.Schema, strings.TrimSpace(in.Name), in.With); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "sequence": in.Name, "restarted": true})
}

// handlePgSequenceSetval POST {app, name, value, is_called?}.
func (d *Deps) handlePgSequenceSetval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpPgWrite) {
		return
	}
	var in struct {
		App      string `json:"app"`
		Name     string `json:"name"`
		Value    int64  `json:"value"`
		IsCalled *bool  `json:"is_called"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	called := true
	if in.IsCalled != nil {
		called = *in.IsCalled
	}
	db, rec, err := d.pgAppDB(strings.TrimSpace(in.App))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out, err := pg.SetvalExec(ctx, db, rec.Schema, strings.TrimSpace(in.Name), in.Value, called)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"app": in.App, "sequence": in.Name, "value": out})
}
