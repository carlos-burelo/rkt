// Detección de plan sin crear la app (preview estilo Vercel).
package api

import (
	"net/http"
	"os"
	"path/filepath"
	"rkt/internal/authz"
	"strings"

	"rkt/internal/buildpack"
	"rkt/internal/discover"
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
	Evidence  []string `json:"evidence,omitempty"`
	Node      string   `json:"node_version,omitempty"`
	Workspace bool     `json:"workspace,omitempty"`
	// Reason por qué no hay plan (nogit/noclone/noplan); "" si hay plan.
	Reason string `json:"reason,omitempty"`
	// Detail causa corta (primera línea del error de git, acotada).
	Detail string `json:"detail,omitempty"`
}

// shortErr primera línea del error, acotada: el stderr de git puede ser
// largo y el wizard solo enseña la causa.
func shortErrStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// handleDetect GET /api/v1/detect?path=<dir> | ?repo=<url> (operator+).
// path inspecciona un directorio local; repo detecta sin clonar completo
// (manifiestos por sparse-checkout, cualquier host). Repo sin plan
// detectable responde 200 con framework vacío: el wizard pide el comando.
func (d *Deps) handleDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpDetectRead) {
		return
	}
	if repo := r.URL.Query().Get("repo"); repo != "" {
		sug, err := discover.Suggest(d.DB, d.githubManager(), repo)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		p := sug.Plan
		out := detectOut{
			Framework: p.Kind,
			PM:        p.PM,
			Install:   nonNil(p.Install),
			Build:     nonNil(p.Build),
			Start:     nonNil(p.Start),
			StaticDir: p.StaticDir,
			Evidence:  p.Evidence,
			Node:      p.NodeVersion,
			Workspace: p.Workspace,
		}
		// Sin plan el wizard no adivina: motivo + causa para el mensaje.
		if p.Kind == "" && sug.Reason != "" {
			out.Reason = sug.Reason
			out.Detail = shortErrStr(sug.Detail, 200)
		}
		writeJSON(w, out)
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
		Evidence:  plan.Evidence,
		Node:      plan.NodeVersion,
		Workspace: plan.Workspace,
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
