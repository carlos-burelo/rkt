package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"rkt/internal/authz"
	"rkt/internal/domains"
	"rkt/internal/store"
)

// handleDomainGuide GET /api/v1/domains/guide?host= (viewer+): qué registro
// DNS crear, en qué estado está y qué sigue. Lógica en domains.BuildGuide.
func (d *Deps) handleDomainGuide(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpDomainsRead) {
		return
	}
	host := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("host")))
	if host == "" || strings.ContainsAny(host, " /\\") {
		writeErr(w, http.StatusBadRequest, "host inválido")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	writeJSON(w, domains.BuildGuide(host,
		d.DB.GetSetting(store.KeyBaseDomain), d.DB.GetSetting(store.KeyPublicIP), d.resolveDNS(ctx, host)))
}
