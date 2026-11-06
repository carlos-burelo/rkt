package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rkt-proxy/internal/netutil"
)

// handleTokens lista o crea tokens de API (admin).
func (d *Deps) handleTokens(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := d.DB.ListTokens()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, list)
	case http.MethodPost:
		var in struct {
			Name     string `json:"name"`
			Role     string `json:"role"`
			TTLHours int    `json:"ttl_hours"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if in.TTLHours <= 0 || in.TTLHours > 24*365 {
			in.TTLHours = 24 * 90
		}
		tok, err := d.DB.CreateToken(in.Name, in.Role, time.Duration(in.TTLHours)*time.Hour)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = d.DB.AddAudit(mustUser(r), "token.create", in.Name, netutil.ClientIP(r))
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"name": in.Name, "role": in.Role, "token": tok})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	}
}

// handleToken revoca /tokens/{id}.
func (d *Deps) handleToken(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	raw := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/"), "/")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
		return
	}
	if err := d.DB.RevokeToken(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	_ = d.DB.AddAudit(mustUser(r), "token.revoke", strconv.FormatInt(id, 10), netutil.ClientIP(r))
	writeJSON(w, map[string]string{"ok": "true"})
}
