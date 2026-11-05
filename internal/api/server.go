package api

import (
	"net/http"
)

// handleServer snapshot local de la instancia.
func (d *Deps) handleServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if d.Server == nil {
		writeErr(w, http.StatusServiceUnavailable, "monitor de servidor no disponible")
		return
	}
	s, err := d.Server.Snapshot()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, s)
}

// handleServerHistory serie corta en memoria.
func (d *Deps) handleServerHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if d.Server == nil {
		writeErr(w, http.StatusServiceUnavailable, "monitor de servidor no disponible")
		return
	}
	writeJSON(w, d.Server.History(queryInt(r, "hours", 6)))
}

// handleServerProcesses top por CPU/RAM.
func (d *Deps) handleServerProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if d.Server == nil {
		writeErr(w, http.StatusServiceUnavailable, "monitor de servidor no disponible")
		return
	}
	writeJSON(w, d.Server.Processes(queryInt(r, "limit", 10)))
}
