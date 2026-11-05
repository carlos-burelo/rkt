// Package dnscheck resuelve hosts de dominio para el diagnóstico del proxy.
// Solo lectura; no toca el resolver del sistema salvo por la interfaz.
package dnscheck

import (
	"context"
	"net"
	"sort"
	"strings"
)

// Resolver mínima que usa Check. net.Resolver la cumple.
type Resolver interface {
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// Result registros observados para un host.
type Result struct {
	A        []string `json:"a"`
	AAAA     []string `json:"aaaa"`
	CNAME    string   `json:"cname,omitempty"`
	Resolves bool     `json:"resolves"`
	Error    string   `json:"error,omitempty"`
}

// Check consulta A/AAAA/CNAME. Un fallo de DNS no es error fatal del
// diagnóstico: se devuelve en Error con Resolves=false.
func Check(ctx context.Context, r Resolver, host string) Result {
	host = strings.TrimSpace(strings.ToLower(host))
	out := Result{A: []string{}, AAAA: []string{}}
	if host == "" {
		out.Error = "host vacío"
		return out
	}
	if r == nil {
		r = net.DefaultResolver
	}
	ips, err := r.LookupIP(ctx, "ip", host)
	if err != nil {
		out.Error = err.Error()
	} else {
		for _, ip := range ips {
			if v4 := ip.To4(); v4 != nil {
				out.A = append(out.A, v4.String())
				continue
			}
			out.AAAA = append(out.AAAA, ip.String())
		}
		out.Resolves = len(out.A)+len(out.AAAA) > 0
	}
	sort.Strings(out.A)
	sort.Strings(out.AAAA)
	if cname, err := r.LookupCNAME(ctx, host); err == nil {
		cname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")
		if cname != "" && cname != host {
			out.CNAME = cname
		}
	}
	return out
}

// Expected IP esperada para un host local o para el dominio propio.
// Vacío = no hay expectativa configurada (no se emite veredicto).
func Expected(host, baseDomain, publicIP string) string {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".test") {
		return "127.0.0.1"
	}
	base := strings.Trim(strings.ToLower(strings.TrimSpace(baseDomain)), ".")
	if base != "" && (h == base || strings.HasSuffix(h, "."+base)) {
		return strings.TrimSpace(publicIP)
	}
	return strings.TrimSpace(publicIP)
}

// Matches true si alguna IP observada coincide con la esperada.
func (r Result) Matches(expected string) bool {
	if expected == "" {
		return r.Resolves
	}
	for _, ip := range r.A {
		if ip == expected {
			return true
		}
	}
	for _, ip := range r.AAAA {
		if ip == expected {
			return true
		}
	}
	return false
}
