package api

import (
	"net/http"
	"strings"
	"time"

	"rkt/internal/authz"
	"rkt/internal/canary"
)

// handleCanary /api/v1/canary:
//
//	GET  lista (operator+)
//	PUT  ?label=&  cuerpo = rkt.exe, cabecera X-Rkt-Sha256 (admin): SOLO sube
func (d *Deps) handleCanary(w http.ResponseWriter, r *http.Request) {
	if d.Canary == nil {
		writeErr(w, http.StatusNotImplemented, "canary no disponible")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !requireOp(w, r, authz.OpCanaryRead) {
			return
		}
		writeJSON(w, d.Canary.List())
	case http.MethodPut:
		if !requireOp(w, r, authz.OpCanaryWrite) {
			return
		}
		body := http.MaxBytesReader(w, r.Body, canary.MaxSize+1)
		m, err := d.Canary.Put(body, r.URL.Query().Get("label"), r.Header.Get("X-Rkt-Sha256"), mustUser(r), time.Now())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"canary": m, "next": "aplica con POST /api/v1/canary/" + m.ID + "/apply (o MCP canary_apply)"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|PUT")
	}
}

// handleCanaryItem /api/v1/canary/{id} (DELETE) y /{id}/apply (POST).
// /api/v1/canary/rollback (POST) vuelve al slot previo.
func (d *Deps) handleCanaryItem(w http.ResponseWriter, r *http.Request) {
	if d.Canary == nil {
		writeErr(w, http.StatusNotImplemented, "canary no disponible")
		return
	}
	if !requireOp(w, r, authz.OpCanaryWrite) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/canary/"), "/")
	switch {
	case rest == "rollback" && r.Method == http.MethodPost:
		if err := canary.Launch(d.CfgPath, d.HomeDir, "--rollback"); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "rollback lanzado; el servicio se reinicia"})
	case strings.HasSuffix(rest, "/apply") && r.Method == http.MethodPost:
		id := strings.TrimSuffix(rest, "/apply")
		_, path, err := d.Canary.Get(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		// --force: el canary puede tener versión menor o igual (dev).
		if err := canary.Launch(d.CfgPath, d.HomeDir, "--local", path, "--force"); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "upgrade lanzado; el servicio se reinicia y vuelve en ~1 min. Consulta canary_status"})
	case r.Method == http.MethodDelete && canary.ValidID(rest):
		if err := d.Canary.Remove(rest); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	default:
		writeErr(w, http.StatusNotFound, "usa DELETE /canary/{id}, POST /canary/{id}/apply o POST /canary/rollback")
	}
}
