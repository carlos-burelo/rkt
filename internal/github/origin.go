package github

import (
	"context"
	"strings"
	"time"

	"rkt/internal/dnscheck"
	"rkt/internal/domains"
	"rkt/internal/netutil"
)

// Plan origen público para el manifest de GitHub App más el veredicto de
// entorno. Env.Blocked() = no crear la App: fallaría (IP pública sin
// dominio, dominio sin A hacia este servidor, base_domain que es una IP).
type Plan struct {
	Origin string
	Domain string
	Local  bool
	TLS    bool
	Env    domains.Env
}

// PlanSetup calcula origen + entorno. Solo consulta DNS cuando hay IP
// pública y un dominio real (r nil = resolver del sistema).
//
// Hogar único: lo usan el servicio (setup_url) y el MCP (chequeo).
func PlanSetup(baseDomain, publicIP, listenHTTP, listenHTTPS string, r dnscheck.Resolver) Plan {
	origin, domain, local, tls := setupOrigin(baseDomain, listenHTTP, listenHTTPS)
	var a []string
	base := domains.CleanBase(baseDomain)
	if base != "" && !domains.IsLocal(base) && domains.Public(publicIP) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		a = dnscheck.Check(ctx, r, base).A
	}
	return Plan{Origin: origin, Domain: domain, Local: local, TLS: tls,
		Env: domains.Classify(baseDomain, publicIP, a)}
}

// setupOrigin sin base_domain cae a loopback (GitHub permite crear Apps
// con URLs locales; sin webhooks, el deploy usa polling).
//
// Ojo: 127.0.0.1 y no "localhost": GitHub rechaza redirect_url con
// localhost + puerto explícito ("must be a valid URL"); con 127.0.0.1
// el manifest pasa. Además coincide con el listener loopback.
func setupOrigin(baseDomain, listenHTTP, listenHTTPS string) (origin, domain string, local, tls bool) {
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
