package api

import (
	"net/http"
	"net/url"
	"strings"

	"rkt-proxy/internal/github"
)

// handleGithub estado vivo de la GitHub App.
func (d *Deps) handleGithub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r) {
		return
	}
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
}

// handleGithubRepos repos accesibles a la App (selector de "nueva app").
func (d *Deps) handleGithubRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r) {
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
