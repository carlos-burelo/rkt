package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dotenv"
)

// handleEnvIndex GET /api/v1/env: claves globales y por app, sin valores.
func (d *Deps) handleEnvIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r) {
		return
	}
	cfg := d.Live.Get()
	global := make([]string, 0, len(cfg.GlobalEnv))
	for k := range cfg.GlobalEnv {
		global = append(global, k)
	}
	sort.Strings(global)
	apps := make(map[string][]string, len(cfg.Apps))
	for _, a := range cfg.Apps {
		keys := make([]string, 0, len(a.Env))
		for k := range a.Env {
			if k == "PORT" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		apps[a.Name] = keys
	}
	writeJSON(w, map[string]any{"global": global, "apps": apps})
}

// handleEnvSub enruta /env/global, /env/global/import y /env/global/{key}.
func (d *Deps) handleEnvSub(w http.ResponseWriter, r *http.Request) {
	if !requireOperator(w, r) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/env/"), "/")
	switch {
	case rest == "global":
		d.handleGlobalEnv(w, r)
	case rest == "global/import":
		d.handleEnvImport(w, r, "")
	case strings.HasPrefix(rest, "global/"):
		d.handleEnvValue(w, r, strings.TrimPrefix(rest, "global/"), "")
	default:
		writeErr(w, http.StatusNotFound, "ruta de entorno inválida")
	}
}

// handleGlobalEnv PUT /env/global {set,unset}.
func (d *Deps) handleGlobalEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeErr(w, http.StatusMethodNotAllowed, "solo PUT")
		return
	}
	set, unset, ok := decodeEnvPatch(w, r)
	if !ok {
		return
	}
	if err := d.mutateInfra(false, func(cp *config.Config) error {
		return cfgfile.ApplyEnvPatch(cp, "", set, unset)
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "restart": true})
}

// handleEnvValue GET /env/global/{key} o /apps/{name}/env/{key}.
func (d *Deps) handleEnvValue(w http.ResponseWriter, r *http.Request, key, app string) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.Contains(key, "/") {
		writeErr(w, http.StatusBadRequest, "clave inválida")
		return
	}
	cfg := d.Live.Get()
	if app == "" {
		if v, ok := cfg.GlobalEnv[key]; ok {
			writeJSON(w, map[string]string{"key": key, "value": v})
			return
		}
		writeErr(w, http.StatusNotFound, "clave inexistente: "+key)
		return
	}
	for _, a := range cfg.Apps {
		if a.Name != app {
			continue
		}
		if v, ok := a.Env[key]; ok {
			writeJSON(w, map[string]string{"key": key, "value": v})
			return
		}
		writeErr(w, http.StatusNotFound, "clave inexistente: "+key)
		return
	}
	writeErr(w, http.StatusNotFound, "app inexistente")
}

// handleEnvImport POST {raw} con el parser canónico de dotenv.
func (d *Deps) handleEnvImport(w http.ResponseWriter, r *http.Request, app string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	var in struct {
		Raw string `json:"raw"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	parsed, err := dotenv.Parse(in.Raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := func() error {
		if app == "" {
			return d.mutateInfra(false, func(cp *config.Config) error {
				return cfgfile.ApplyEnvPatch(cp, "", parsed, nil)
			})
		}
		return d.mutateFleet(false, func(cp *config.Config) error {
			return cfgfile.ApplyEnvPatch(cp, app, parsed, nil)
		})
	}(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeJSON(w, map[string]any{"ok": true, "keys": keys})
}

func decodeEnvPatch(w http.ResponseWriter, r *http.Request) (map[string]string, []string, bool) {
	var in struct {
		Set   map[string]string `json:"set"`
		Unset []string          `json:"unset"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return nil, nil, false
	}
	if in.Set == nil {
		in.Set = map[string]string{}
	}
	if len(in.Set) == 0 && len(in.Unset) == 0 {
		writeErr(w, http.StatusBadRequest, "nada que aplicar (set/unset vacíos)")
		return nil, nil, false
	}
	return in.Set, in.Unset, true
}

// envKeys devuelve claves visibles de una app (sin PORT).
func envKeys(appENV map[string]string) []string {
	out := make([]string, 0, len(appENV))
	for k := range appENV {
		if k == "PORT" {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// patchAppEnv aplica set/unset sobre una app.
func (d *Deps) patchAppEnv(w http.ResponseWriter, r *http.Request, app string) {
	set, unset, ok := decodeEnvPatch(w, r)
	if !ok {
		return
	}
	if err := d.mutateFleet(false, func(cp *config.Config) error {
		return cfgfile.ApplyEnvPatch(cp, app, set, unset)
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "restart": true})
}
