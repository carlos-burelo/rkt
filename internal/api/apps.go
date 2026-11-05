package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"rkt-proxy/internal/appusage"
	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/store"
)

// handleApps lista la flota o crea una app (GET/POST).
func (d *Deps) handleApps(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		live := d.Live.Get()
		out := []map[string]any{}
		for _, x := range live.Apps {
			out = append(out, appview.App(live, x))
		}
		writeJSON(w, out)
		return
	}
	if r.Method == http.MethodPost {
		if !requireOperator(w, r) {
			return
		}
		var in struct {
			Repo    string `json:"repo"`
			Name    string `json:"name"`
			Runtime string `json:"runtime"`
			Domain  string `json:"domain"`
			Port    int    `json:"port"`
			Command string `json:"command"`
			Dir     string `json:"dir"`
			Auto    bool   `json:"auto"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		name, err := createAppFromRepo(d.DB, d.Live, d.githubManager(), in.Runtime, in.Repo, in.Name, in.Domain, in.Port, in.Command, in.Dir, in.Auto)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"app": name})
		return
	}
	writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
}

// handleAppDetail enruta /api/v1/apps/<name>[/<action>].
func (d *Deps) handleAppDetail(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/apps/")
	name, action := rest, ""
	if i := strings.Index(rest, "/"); i >= 0 {
		name, action = rest[:i], rest[i+1:]
	}
	a := cfgfile.FindApp(d.Live.Get(), name)
	if a == nil {
		writeErr(w, http.StatusNotFound, "app inexistente")
		return
	}
	if a.External() && (action == "deploy" || action == "restart" || action == "rollback" || action == "build" || action == "retry") {
		writeErr(w, http.StatusConflict, "app externa: solo proxy; corre el dev server en "+a.Dir)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, appview.App(d.Live.Get(), *a))
	case action == "usage" && r.Method == http.MethodGet:
		st := health.AppStatus{Name: name}
		if d.Apps != nil {
			for _, s := range d.Apps() {
				if s.Name == name {
					st = s
				}
			}
		}
		win := queryInt(r, "window", 300)
		if win > 86400 {
			win = 86400
		}
		rep, err := appusage.Collect(appusage.Input{
			App: *a, Cfg: d.Live.Get(), LogsDir: d.LogsDir,
			Status: st, Window: time.Duration(win) * time.Second,
		})
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, rep)
	case action == "" && r.Method == http.MethodPut:
		if !requireOperator(w, r) {
			return
		}
		var in cfgfile.AppPatch
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		cp := d.Live.Get().Clone()
		if err := cfgfile.UpdateApp(cp, name, in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp)
		if d.Apply != nil {
			d.Apply(cp.Apps, cp.GlobalEnv)
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "" && r.Method == http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		cp := d.Live.Get().Clone()
		dropped, err := cfgfile.RemoveApp(cp, name, false)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp)
		if d.Apply != nil {
			d.Apply(cp.Apps, cp.GlobalEnv)
		}
		writeJSON(w, map[string]any{
			"ok": "true", "routes_dropped": dropped,
			"hint": "rutas de la app retiradas; revisa huérfanas: routes list",
		})
	case action == "build" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		if d.Build == nil {
			writeErr(w, http.StatusServiceUnavailable, "build deshabilitado")
			return
		}
		id, err := d.Build(name, mustUser(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"deployment": id})
	case action == "restart" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		if d.Restart == nil || !d.Restart(name) {
			writeErr(w, http.StatusNotFound, "restart fallido")
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "deploy" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		if r.URL.Query().Get("async") == "1" && d.DeployAsync != nil {
			id, err := d.DeployAsync(name, mustUser(r))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusAccepted)
			writeJSON(w, map[string]any{"deployment": id})
			return
		}
		if d.Deploy == nil {
			writeErr(w, http.StatusServiceUnavailable, "deploy deshabilitado")
			return
		}
		rev, err := d.Deploy(name, mustUser(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"rev": rev})
	case action == "rollback" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		if d.Rollback == nil {
			writeErr(w, http.StatusServiceUnavailable, "rollback deshabilitado")
			return
		}
		if err := d.Rollback(name, mustUser(r)); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "env" && r.Method == http.MethodGet:
		if !requireOperator(w, r) {
			return
		}
		env := a.Env
		if env == nil {
			env = map[string]string{}
		}
		writeJSON(w, env)
	case action == "env" && r.Method == http.MethodPut:
		if !requireOperator(w, r) {
			return
		}
		var in struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if strings.TrimSpace(in.Key) == "" {
			writeErr(w, http.StatusBadRequest, "key vacía")
			return
		}
		cp := d.Live.Get().Clone()
		if err := cfgfile.SetEnv(d.DB, cp, name, strings.TrimSpace(in.Key), in.Value); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp) // inmediato (watchMerged lo reconfirma)
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "env" && r.Method == http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		key := r.URL.Query().Get("key")
		if key == "" {
			writeErr(w, http.StatusBadRequest, "falta ?key=")
			return
		}
		cp := d.Live.Get().Clone()
		if err := cfgfile.UnsetEnv(cp, name, key); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := cfgfile.SaveFleet(d.DB, cp); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp) // inmediato (watchMerged lo reconfirma)
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusNotFound, "ruta inválida")
	}
}

// createAppFromRepo alta con inferencia vía cfgfile.CreateAndAdd (hogar
// único, también usado por MCP). Refresca la config viva.
func createAppFromRepo(db *store.DB, live *config.Live, gh *github.Manager, runtime, repo, name, domain string, port int, command, dir string, auto bool) (string, error) {
	app, _, _, err := cfgfile.CreateAndAdd(db, gh, runtime, repo, name, domain, port, command, dir, auto)
	if err != nil {
		return "", err
	}
	if snap, err := cfgfile.Snapshot(db, live.Get()); err == nil {
		live.Swap(snap)
	}
	return app.Name, nil
}
