package api

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
)

// handleDeployments lista el historial (GET).
func (d *Deps) handleDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	n := queryInt(r, "n", 20)
	deps, err := d.DB.ListDeployments(r.URL.Query().Get("app"), n)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, deps)
}

// handleEvents lista la auditoría del supervisor (GET).
func (d *Deps) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	q := r.URL.Query()
	evs, err := d.DB.ListEvents(queryInt(r, "n", 20), q.Get("kind"), q.Get("app"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, evs)
}

// logPath resuelve el log operativo o el de una app validada.
func (d *Deps) logPath(r *http.Request) (string, bool) {
	path := filepath.Join(d.LogsDir, "rkt.log")
	if app := r.URL.Query().Get("app"); app != "" {
		if !config.ValidAppName(app) {
			return "", false
		}
		path = filepath.Join(d.LogsDir, "apps", app+".log")
	}
	return path, true
}

// handleLogs tail del log operativo o de app (GET).
func (d *Deps) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	path, ok := d.logPath(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "app inválida [a-z0-9-]")
		return
	}
	out, err := logx.Tail(path, queryInt(r, "n", 50))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"log": out})
}

// handleLogStream mismo log en vivo por SSE (GET).
func (d *Deps) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	path, ok := d.logPath(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "app inválida [a-z0-9-]")
		return
	}
	streamLog(w, r, path, queryInt(r, "n", 200), nil)
}

// handleDeploymentDetail sirve /deployments/<id>, /<id>/log y /<id>/retry.
func (d *Deps) handleDeploymentDetail(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
	parts := strings.Split(rest, "/")
	if len(parts) == 2 && parts[1] == "retry" && r.Method == http.MethodPost {
		if !requireOperator(w, r) {
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 || d.Retry == nil {
			writeErr(w, http.StatusBadRequest, "id inválido o retry deshabilitado")
			return
		}
		rev, err := d.Retry(id, mustUser(r))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"rev": rev})
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		deps, err := d.DB.ListDeployments("", 2000)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, dep := range deps {
			if dep.ID == id {
				writeJSON(w, dep)
				return
			}
		}
		writeErr(w, http.StatusNotFound, "deployment inexistente")
		return
	}
	if len(parts) != 2 || parts[1] != "log" || r.Method != http.MethodGet {
		writeErr(w, http.StatusNotFound, "ruta inválida (…/ID/log|…/ID/retry)")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	_, lp, ok := d.DB.DeploymentLogPath(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "sin log para ese deploy")
		return
	}
	if r.URL.Query().Get("stream") == "1" {
		streamLog(w, r, lp, queryInt(r, "n", 200), func() bool {
			st, ok := d.DB.DeploymentStatus(id)
			return !ok || st != "running"
		})
		return
	}
	out, err := logx.Tail(lp, queryInt(r, "n", 200))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"log": out})
}
