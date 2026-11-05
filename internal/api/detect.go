// Detección de plan sin crear la app (preview estilo Vercel).
package api

import (
	"net/http"
	"os"
	"path/filepath"

	"rkt-proxy/internal/buildpack"
)

type detectOut struct {
	Path      string   `json:"path"`
	Framework string   `json:"framework"`
	PM        string   `json:"pm,omitempty"`
	Install   []string `json:"install"`
	Build     []string `json:"build"`
	Start     []string `json:"start"`
	StaticDir string   `json:"static_dir,omitempty"`
	StaticAbs string   `json:"static_abs,omitempty"`
}

// handleDetect GET /api/v1/detect?path=<dir> (operator+).
func (d *Deps) handleDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r) {
		return
	}
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "falta path")
		return
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ruta inválida")
		return
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		writeErr(w, http.StatusNotFound, "directorio no existe: "+abs)
		return
	}
	plan, err := buildpack.DetectDir(abs)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	out := detectOut{
		Path:      abs,
		Framework: plan.Kind,
		PM:        plan.PM,
		Install:   nonNil(plan.Install),
		Build:     nonNil(plan.Build),
		Start:     nonNil(plan.Start),
		StaticDir: plan.StaticDir,
	}
	if plan.IsStatic() {
		out.StaticAbs = plan.Static(abs)
	}
	writeJSON(w, out)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
