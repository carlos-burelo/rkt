package api

import (
	"net/http"
	"net/url"
	"rkt/internal/authz"
	"strings"

	"rkt/internal/github"
)

// handleGithub estado vivo de la GitHub App o desvinculación.
func (d *Deps) handleGithub(w http.ResponseWriter, r *http.Request) {
	if !requireOp(w, r, authz.OpGitHubWrite) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if d.Github == nil {
			writeJSON(w, map[string]any{"configured": false})
			return
		}
		st, err := d.Github()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		// ?org= instala en una organización: se propaga al form new-app.
		if org := strings.TrimSpace(r.URL.Query().Get("org")); org != "" {
			if u, ok := st["setup_url"].(string); ok && u != "" {
				sep := "&"
				if !strings.Contains(u, "?") {
					sep = "?"
				}
				st["setup_url"] = u + sep + "org=" + url.QueryEscape(org)
			}
		}
		writeJSON(w, st)
	case http.MethodDelete:
		if d.GithubDisconnect == nil {
			writeErr(w, http.StatusNotImplemented, "desconexión no disponible")
			return
		}
		if err := d.GithubDisconnect(); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "GitHub desvinculado"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "solo GET o DELETE")
	}
}

// handleGithubTest prueba en vivo la conexión con GitHub.
func (d *Deps) handleGithubTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpGitHubWrite) {
		return
	}
	if d.GithubTest == nil {
		writeErr(w, http.StatusNotImplemented, "prueba no disponible")
		return
	}
	res, err := d.GithubTest()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, res)
}

// handleGithubRepos repos accesibles a la App (selector de "nueva app").
func (d *Deps) handleGithubRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpGitHubWrite) {
		return
	}
	if d.GithubRepos == nil {
		writeJSON(w, []github.Repo{})
		return
	}
	repos, err := d.GithubRepos()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, repos)
}
