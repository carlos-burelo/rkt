package api

import (
	"net/http"

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
