package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
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
		if err := d.mutateInfra(true, func(cp *config.Config) error {
			return cfgfile.SetGlobalEnvValue(cp, key, in.Value)
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true", "hint": "las apps heredan la global al reiniciar"})
	case http.MethodDelete:
		key := r.URL.Query().Get("key")
		if key == "" {
			writeErr(w, http.StatusBadRequest, "falta ?key=")
			return
		}
		if err := d.mutateInfra(true, func(cp *config.Config) error {
			cfgfile.UnsetGlobalEnvValue(cp, key)
			return nil
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "método no permitido")
	}
}
