// Endpoint de hosts locales: arregla el DNS de dominios *.test/base desde
// la UI (requiere que el servicio corra con permisos de administrador).
package api

import (
	"encoding/json"
	"net/http"

	"rkt-proxy/internal/hosts"
)

// handleHosts GET lista, POST {host} añade, DELETE ?host= quita.
func (d *Deps) handleHosts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := hosts.List()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"hosts": list, "path": hosts.Path()})
	case http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		var in struct {
			Host string `json:"host"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := hosts.Add(in.Host); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
				"hint":  "ejecuta como administrador: rkt hosts add " + in.Host,
			})
			return
		}
		writeJSON(w, map[string]string{"ok": "true", "host": in.Host})
	case http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		host := r.URL.Query().Get("host")
		if err := hosts.Remove(host); err != nil {
			writeErr(w, http.StatusForbidden, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST|DELETE")
	}
}
