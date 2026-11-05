package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// handleAlerts lista reglas (viewer) o crea una (operator).
func (d *Deps) handleAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := d.DB.ListRules()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, rules)
	case http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		var in struct {
			App       string  `json:"app"`
			Kind      string  `json:"kind"`
			Threshold float64 `json:"threshold"`
			WindowMin int     `json:"window_min"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		id, err := d.DB.AddRule(in.App, in.Kind, in.Threshold, in.WindowMin)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]int64{"id": id})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	}
}

// handleAlert borra una regla por id (operator).
func (d *Deps) handleAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
		return
	}
	if !requireOperator(w, r) {
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v1/alerts/"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	if err := d.DB.DeleteRule(id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}
