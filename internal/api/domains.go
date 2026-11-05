package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/dnscheck"
	"rkt-proxy/internal/store"
)

type domainTLS struct {
	Managed bool   `json:"managed"`
	Expires string `json:"expires,omitempty"`
}

type domainDetail struct {
	Host      string          `json:"host"`
	Target    string          `json:"target"`
	App       string          `json:"app,omitempty"`
	TLS       domainTLS       `json:"tls"`
	DNS       dnscheck.Result `json:"dns"`
	Expected  string          `json:"expected,omitempty"`
	DNSOK     bool            `json:"dns_ok"`
	CheckedAt string          `json:"checked_at"`
}

// handleDomainPreflight GET /api/v1/domains/preflight?host=: DNS antes de
// crear una ruta. No exige que la ruta exista.
func (d *Deps) handleDomainPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	host := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("host")))
	if host == "" || strings.ContainsAny(host, " /\\") {
		writeErr(w, http.StatusBadRequest, "host inválido")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dns := d.DNS(ctx, host)
	expected := dnscheck.Expected(host, d.DB.GetSetting(store.KeyBaseDomain), d.DB.GetSetting(store.KeyPublicIP))
	writeJSON(w, map[string]any{
		"host": host, "dns": dns, "expected": expected, "dns_ok": dns.Matches(expected),
	})
}

// handleDomainDetail GET /api/v1/domains/{host}: vista agregada para el
// drawer de Dominios. No escribe ni emite certificados.
func (d *Deps) handleDomainDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	host := strings.TrimSpace(strings.ToLower(strings.TrimPrefix(r.URL.Path, "/api/v1/domains/")))
	if host == "" || strings.Contains(host, "/") {
		writeErr(w, http.StatusNotFound, "ruta inválida (usa /domains/HOST)")
		return
	}
	cfg := d.Live.Get()
	target := ""
	for _, route := range cfg.Routes {
		for _, h := range route.Hosts {
			if strings.EqualFold(strings.TrimSpace(h), host) {
				target = route.Target
				break
			}
		}
		if target != "" {
			break
		}
	}
	if target == "" {
		writeErr(w, http.StatusNotFound, "dominio sin ruta: "+host)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dns := d.DNS(ctx, host)
	expected := dnscheck.Expected(host, d.DB.GetSetting(store.KeyBaseDomain), d.DB.GetSetting(store.KeyPublicIP))
	managed, expires := false, ""
	if d.TLS != nil {
		managed, expires = d.TLS(host)
	}
	writeJSON(w, domainDetail{
		Host: host, Target: target, App: appview.Owner(cfg, target),
		TLS:       domainTLS{Managed: managed, Expires: expires},
		DNS:       dns,
		Expected:  expected,
		DNSOK:     dns.Matches(expected),
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
