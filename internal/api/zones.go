// Zonas locales: agrupa rutas por sufijo local (localhost, test, base)
// para gestionarlas como un panel DNS (dominio base + subdominios).
package api

import (
	"net/http"
	"sort"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

type zoneHost struct {
	Host   string `json:"host"`
	Target string `json:"target"`
	IsBase bool   `json:"is_base"`
}

type zoneGroup struct {
	Base   string     `json:"base"`
	Suffix string     `json:"suffix"`
	Target string     `json:"target"`
	Hosts  []zoneHost `json:"hosts"`
}

type zonesOut struct {
	Suffixes []string    `json:"suffixes"`
	Groups   []zoneGroup `json:"groups"`
}

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
	writeJSON(w, buildZones(d.Live.Get().Routes, base))
}

// buildZones puro: agrupa hosts locales en dominio base + subdominios.
func buildZones(routes []config.Route, customBase string) zonesOut {
	suffixes := []string{"localhost", "test"}
	if b := strings.Trim(strings.TrimSpace(customBase), "."); b != "" && b != "localhost" && b != "test" {
		suffixes = append(suffixes, b)
	}
	target := map[string]string{}
	for _, rt := range routes {
		for _, h := range rt.Hosts {
			target[strings.ToLower(h)] = rt.Target
		}
	}

	groups := map[string]*zoneGroup{}
	for host, tgt := range target {
		suffix, rest := splitLocal(host, suffixes)
		if suffix == "" {
			continue
		}
		base := host
		if strings.Contains(suffix, ".") {
			// zona propia (base_domain): el apex es el sufijo
			base = suffix
		} else if i := strings.Index(rest, "."); i >= 0 {
			base = rest[i+1:] + "." + suffix
		}
		g, ok := groups[base]
		if !ok {
			g = &zoneGroup{Base: base, Suffix: suffix, Hosts: []zoneHost{}}
			groups[base] = g
		}
		if host == base {
			g.Target = tgt
		}
		g.Hosts = append(g.Hosts, zoneHost{Host: host, Target: tgt, IsBase: host == base})
	}

	out := zonesOut{Suffixes: suffixes, Groups: []zoneGroup{}}
	for _, g := range groups {
		sort.Slice(g.Hosts, func(i, j int) bool {
			if g.Hosts[i].IsBase != g.Hosts[j].IsBase {
				return g.Hosts[i].IsBase
			}
			return g.Hosts[i].Host < g.Hosts[j].Host
		})
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Base < out.Groups[j].Base })
	return out
}

// splitLocal devuelve el sufijo de zona y el resto (subdominio[.base]).
func splitLocal(host string, suffixes []string) (suffix, rest string) {
	for _, s := range suffixes {
		if strings.HasSuffix(host, "."+s) {
			return s, strings.TrimSuffix(host, "."+s)
		}
	}
	return "", ""
}
