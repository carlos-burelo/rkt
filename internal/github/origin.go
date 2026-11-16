package github

import (
	"strings"

	"rkt/internal/netutil"
)

// SetupOrigin devuelve el origen público para el manifest de GitHub App.
// Sin base_domain cae a loopback (GitHub permite crear Apps con URLs
// locales; sin webhooks, el deploy usa polling).
//
// Ojo: 127.0.0.1 y no "localhost": GitHub rechaza redirect_url con
// localhost + puerto explícito ("must be a valid URL"); con 127.0.0.1
// el manifest pasa. Además coincide con el listener loopback.
//
// Hogar único: lo usan el servicio (setup_url) y el MCP (chequeo).
func SetupOrigin(baseDomain, listenHTTP, listenHTTPS string) (origin, domain string, local, tls bool) {
	base := strings.TrimSpace(baseDomain)
	if base != "" {
		return "https://" + base, base, false, true
	}
	tls = listenHTTPS != ""
	addr := listenHTTP
	if tls {
		addr = listenHTTPS
	}
	port := netutil.Port(addr, "")
	host := "127.0.0.1"
	if (tls && port != "" && port != "443") || (!tls && port != "" && port != "80") {
		host += ":" + port
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	return scheme + "://" + host, host, true, tls
}
