package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"rkt/internal/authz"
	"rkt/internal/geoip"
)

// handleAttacks GET /api/v1/security/attacks?hours=24 (operator+): agregados
// del WAF por país, IP, regla y hora, más los últimos bloqueos. Base del
// mapa de ataques. geoip_ready=false indica países sin resolver ("??").
func (d *Deps) handleAttacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOp(w, r, authz.OpWAFRead) {
		return
	}
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 || hours > 24*30 {
		hours = 24
	}
	sum, err := d.DB.SummarizeAttacks(hours, 15, time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"summary": sum, "geoip_ready": d.GeoIP != nil && d.GeoIP.Ready()})
}

// handleGeoIPUpdate POST /api/v1/security/geoip (operator+): descarga la
// base país por IP (DB-IP lite, ~5 MB) y la deja lista sin reiniciar.
func (d *Deps) handleGeoIPUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	if !requireOp(w, r, authz.OpWAFWrite) {
		return
	}
	if d.GeoIP == nil {
		writeErr(w, http.StatusNotImplemented, "geoip no disponible")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	src, err := geoip.Update(ctx, d.GeoIP.Path(), time.Now())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "descarga geoip falló: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "source": src})
}
