package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/waf"
)

// handleWAF reglas y bloqueos recientes (operator: expone IPs atacantes).
// GET lista; PUT reemplaza deny_paths y/o deny_methods (validado).
func (d *Deps) handleWAF(w http.ResponseWriter, r *http.Request) {
	if !requireOperator(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg := d.Live.Get()
		writeJSON(w, map[string]any{
			"deny_paths":   append([]string(nil), cfg.DenyPaths...),
			"deny_methods": append([]string(nil), cfg.DenyMethods...),
			"blocked":      waf.Recent(),
		})
	case http.MethodPut:
		var in struct {
			DenyPaths   []string `json:"deny_paths"`
			DenyMethods []string `json:"deny_methods"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
		if err := dec.Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		// normalize: métodos en mayúsculas, todo recortado
		methods := make([]string, 0, len(in.DenyMethods))
		for _, m := range in.DenyMethods {
			methods = append(methods, strings.ToUpper(strings.TrimSpace(m)))
		}
		var paths []string
		if in.DenyPaths != nil {
			paths = make([]string, 0, len(in.DenyPaths))
			for _, p := range in.DenyPaths {
				if t := strings.TrimSpace(p); t != "" {
					paths = append(paths, t)
				}
			}
		}
		// MutateInfra valida toda la config (incluye deny_*).
		err := d.mutateInfra(false, func(cp *config.Config) error {
			if in.DenyPaths != nil {
				cp.DenyPaths = paths
			}
			if in.DenyMethods != nil {
				cp.DenyMethods = methods
			}
			return nil
		})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg := d.Live.Get()
		writeJSON(w, map[string]any{
			"deny_paths":   cfg.DenyPaths,
			"deny_methods": cfg.DenyMethods,
		})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|PUT")
	}
}
