// Zonas locales: agrupa rutas por sufijo local (localhost, test, base)
// para gestionarlas como un panel DNS (dominio base + subdominios).
package api

import (
	"net/http"
	"strings"

	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/store"
)

// handleZones GET /api/v1/domains/zones (operator+).
func (d *Deps) handleZones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r, d.DB) {
		return
	}
	base := strings.TrimSpace(d.DB.GetSetting(store.KeyBaseDomain))
	writeJSON(w, appview.BuildZones(d.Live.Get().Routes, base))
}
