package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"rkt/internal/authz"
	"rkt/internal/deploy"
	"rkt/internal/gitcred"
	"rkt/internal/logx"
)

// handleGitCreds /api/v1/git/credentials:
//
//	GET  lista metadatos, nunca secretos (operator+)
//	POST {name,kind:pat|ssh,match,secret?} (admin). ssh sin secret genera la
//	     llave en el servidor y devuelve la pública para registrar en GitHub.
func (d *Deps) handleGitCreds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !requireOp(w, r, authz.OpGitRead) {
			return
		}
		list, err := d.DB.ListGitCreds()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, list)
	case http.MethodPost:
		if !requireOp(w, r, authz.OpGitWrite) {
			return
		}
		var in struct {
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Match  string `json:"match"`
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		res, err := gitcred.Add(d.DB, logx.SecretsDir(d.CfgPath), gitcred.AddReq{Name: in.Name, Kind: in.Kind, Match: in.Match, Secret: in.Secret})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, res)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	}
}

// handleGitCred DELETE /api/v1/git/credentials/{name} (admin).
func (d *Deps) handleGitCred(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
		return
	}
	if !requireOp(w, r, authz.OpGitWrite) {
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/git/credentials/"), "/")
	if err := gitcred.Remove(d.DB, logx.SecretsDir(d.CfgPath), name); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleGitAccess GET /api/v1/git/access?repo=URL (operator+): qué
// mecanismo se usaría para clonar el repo y si realmente alcanza (ls-remote).
func (d *Deps) handleGitAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpGitRead) {
		return
	}
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if repo == "" {
		writeErr(w, http.StatusBadRequest, "falta ?repo=https://github.com/owner/repo")
		return
	}
	if d.GitResolve == nil {
		writeErr(w, http.StatusNotImplemented, "resolución de acceso no disponible")
		return
	}
	acc, err := d.GitResolve(repo)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ctx := r.Context()
	if acc.SSHKey != "" {
		ctx = deploy.WithSSHKey(ctx, acc.SSHKey)
	}
	out := map[string]any{"via": acc.Via, "note": acc.Note, "ok": true}
	if cerr := deploy.CheckAccess(ctx, repo, acc.Token); cerr != nil {
		out["ok"] = false
		out["error"] = cerr.Error()
		out["hint"] = AccessHint(acc.Via)
	}
	writeJSON(w, out)
}

// AccessHint siguiente paso cuando el acceso elegido no alcanza el repo.
func AccessHint(via string) string {
	if via == "anonymous" || via == "env" {
		return "repo privado sin acceso: instala la GitHub App en su dueño (github_revalidate) o guarda un PAT/deploy key (git_cred_add)"
	}
	return "la credencial elegida no alcanza este repo: revisa permisos (PAT: repo/contents; deploy key: registrada en ESE repo)"
}

// handleGithubRevalidate POST /api/v1/github/revalidate (operator+): relee
// las instalaciones de la App y detecta cuentas/orgs sin la App.
func (d *Deps) handleGithubRevalidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpGitHubWrite) {
		return
	}
	if d.GithubRevalidate == nil {
		writeErr(w, http.StatusNotImplemented, "revalidate no disponible")
		return
	}
	rep, err := d.GithubRevalidate()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, rep)
}
