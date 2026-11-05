package api

import (
	"encoding/json"
	"net/http"

	"rkt-proxy/internal/settings"
	"rkt-proxy/internal/store"
)

// handleSettings lectura viewer, escritura operator (solo conocidas).
func (d *Deps) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		out := map[string]string{}
		for _, k := range store.KnownSettings() {
			out[k] = d.DB.GetSetting(k)
		}
		writeJSON(w, out)
	case http.MethodPut:
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
		known := false
		for _, k := range store.KnownSettings() {
			if k == in.Key {
				known = true
			}
		}
		if !known {
			writeErr(w, http.StatusBadRequest, "setting desconocido")
			return
		}
		if err := settings.Validate(in.Key, in.Value); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := d.DB.SetSetting(in.Key, in.Value); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|PUT")
	}
}
