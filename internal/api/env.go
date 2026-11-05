package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"rkt-proxy/internal/cfgfile"
)

// handleGlobalEnv variables compartidas por toda la flota (la app gana).
func (d *Deps) handleGlobalEnv(w http.ResponseWriter, r *http.Request) {
	if !requireOperator(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		env := d.Live.Get().GlobalEnv
		if env == nil {
			env = map[string]string{}
		}
		writeJSON(w, env)
	case http.MethodPut:
		var in struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		key := strings.TrimSpace(in.Key)
		if key == "" {
			writeErr(w, http.StatusBadRequest, "key vacía")
			return
		}
		cp := d.Live.Get().Clone()
		if err := cfgfile.SetGlobalEnv(d.DB, cp, key, in.Value); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp)
		if d.Apply != nil {
			d.Apply(cp.Apps, cp.GlobalEnv)
		}
		writeJSON(w, map[string]string{"ok": "true", "hint": "las apps heredan la global al reiniciar"})
	case http.MethodDelete:
		key := r.URL.Query().Get("key")
		if key == "" {
			writeErr(w, http.StatusBadRequest, "falta ?key=")
			return
		}
		cp := d.Live.Get().Clone()
		if err := cfgfile.UnsetGlobalEnv(d.DB, cp, key); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		d.Live.Swap(cp)
		if d.Apply != nil {
			d.Apply(cp.Apps, cp.GlobalEnv)
		}
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "método no permitido")
	}
}
