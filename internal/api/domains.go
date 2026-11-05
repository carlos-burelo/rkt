package api

import (
	"net/http"
	"strings"
)

// handleDomainTLS estado del certificado de un host (cache ACME).
func (d *Deps) handleDomainTLS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/domains/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[1] != "tls" || parts[0] == "" {
		writeErr(w, http.StatusNotFound, "ruta inválida (…/HOST/tls)")
		return
	}
	managed, expires := false, ""
	if d.TLS != nil {
		managed, expires = d.TLS(parts[0])
	}
	writeJSON(w, map[string]any{"host": parts[0], "managed": managed, "expires": expires})
}
