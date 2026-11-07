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
	"rkt-proxy/internal/deploy"
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
			Repo    string   `json:"repo"`
			Name    string   `json:"name"`
			Runtime string   `json:"runtime"`
			Domains []string `json:"domains"`
			Port    int      `json:"port"`
			Command string   `json:"command"`
			Dir     string   `json:"dir"`
			Auto    bool     `json:"auto"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		d.mu.Lock()
		name, err := createAppFromRepo(d.DB, d.Live, d.githubManager(), in.Runtime, in.Repo, in.Name, in.Domains, in.Port, in.Command, in.Dir, in.Auto)
		d.mu.Unlock()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		// Reconciliar ya: la UI suele lanzar el primer build al crear y el
		// watcher de 10s aún no habría añadido la app al supervisor.
		if d.Apply != nil {
			cfg := d.Live.Get()
			d.Apply(cfg.Apps, cfg.GlobalEnv)
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
	if a.External() && (action == "deploy" || action == "restart" || action == "rollback" || action == "build" || action == "retry" || action == "diff" || action == "start" || action == "stop") {
		writeErr(w, http.StatusConflict, "app externa: solo proxy; corre el dev server en "+a.Dir)
		return
	}
	switch {
	case action == "diff" && r.Method == http.MethodGet:
		if !requireOperator(w, r) {
			return
		}
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		if from == "" || to == "" {
			writeErr(w, http.StatusBadRequest, "from y to requeridos")
			return
		}
		files, err := deploy.DiffFiles(a.Dir, from, to)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, map[string]any{"from": from, "to": to, "files": files})
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
		if err := d.mutateFleet(true, func(cp *config.Config) error {
			return cfgfile.UpdateApp(cp, name, in)
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "" && r.Method == http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		dropped := 0
		if err := d.mutateFleet(true, func(cp *config.Config) error {
			n, err := cfgfile.RemoveApp(cp, name, false)
			dropped = n
			return err
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
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
	case (action == "start" || action == "stop") && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		disabled := action == "stop"
		if err := d.mutateFleet(true, func(cp *config.Config) error {
			return cfgfile.UpdateApp(cp, name, cfgfile.AppPatch{Disabled: &disabled})
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "disabled": disabled})
	case action == "deploy" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		// App local (sin repo): "redesplegar" significa rebuild.
		if a.Deploy == nil {
			if d.Build == nil {
				writeErr(w, http.StatusServiceUnavailable, "build deshabilitado")
				return
			}
			id, err := d.Build(name, mustUser(r))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusAccepted)
			writeJSON(w, map[string]any{"deployment": id})
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
		writeJSON(w, envKeys(a.Env))
	case action == "env" && r.Method == http.MethodPatch:
		d.patchAppEnv(w, r, name)
	case action == "env/import" && r.Method == http.MethodPost:
		d.handleEnvImport(w, r, name)
	case strings.HasPrefix(action, "env/") && r.Method == http.MethodGet:
		d.handleEnvValue(w, r, strings.TrimPrefix(action, "env/"), name)
	default:
		writeErr(w, http.StatusNotFound, "ruta inválida")
	}
}

// createAppFromRepo alta con inferencia vía cfgfile.CreateAndAdd (hogar
// único, también usado por MCP). Refresca la config viva.
func createAppFromRepo(db *store.DB, live *config.Live, gh *github.Manager, runtime, repo, name string, domains []string, port int, command, dir string, auto bool) (string, error) {
	app, _, _, err := cfgfile.CreateAndAdd(db, gh, runtime, repo, name, domains, port, command, dir, auto)
	if err != nil {
		return "", err
	}
	if snap, err := cfgfile.Snapshot(db, live.Get()); err == nil {
		live.Swap(snap)
	}
	return app.Name, nil
}
