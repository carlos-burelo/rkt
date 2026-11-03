package api

import (
	"testing"

	"rkt-proxy/internal/config"
)

func TestBuildZonesAgrupaBaseYSubdominios(t *testing.T) {
	routes := []config.Route{
		{Hosts: []string{"coatl-in.localhost"}, Target: "http://127.0.0.1:3001"},
		{Hosts: []string{"admin.coatl-in.localhost"}, Target: "http://127.0.0.1:3002"},
		{Hosts: []string{"api.test"}, Target: "http://127.0.0.1:4000"},
		{Hosts: []string{"otro.dominio.com"}, Target: "http://127.0.0.1:5000"},
	}
	z := buildZones(routes, "")
	if len(z.Groups) != 2 {
		t.Fatalf("grupos = %+v, quiero 2 (coatl-in.localhost y api.test)", z.Groups)
	}
	if z.Groups[0].Base != "api.test" || z.Groups[1].Base != "coatl-in.localhost" {
		t.Fatalf("bases = %q / %q", z.Groups[0].Base, z.Groups[1].Base)
	}
	g := z.Groups[1]
	if g.Target != "http://127.0.0.1:3001" {
		t.Fatalf("target base = %q", g.Target)
	}
	if len(g.Hosts) != 2 || !g.Hosts[0].IsBase || g.Hosts[1].Host != "admin.coatl-in.localhost" {
		t.Fatalf("hosts = %+v, quiero base primero y subdominio después", g.Hosts)
	}
}

func TestBuildZonesIncluyeBaseDomainComoZona(t *testing.T) {
	z := buildZones([]config.Route{{Hosts: []string{"app.midominio.dev"}, Target: "http://127.0.0.1:3000"}}, "midominio.dev")
	found := false
	for _, s := range z.Suffixes {
		if s == "midominio.dev" {
			found = true
		}
	}
	if !found {
		t.Fatalf("suffixes = %v, quiero incluir base_domain", z.Suffixes)
	}
	if len(z.Groups) != 1 || z.Groups[0].Base != "midominio.dev" {
		t.Fatalf("groups = %+v, quiero base midominio.dev", z.Groups)
	}
}

func TestBuildZonesIgnoraHostsNoLocales(t *testing.T) {
	z := buildZones([]config.Route{{Hosts: []string{"app.ejemplo.com"}, Target: "http://127.0.0.1:3000"}}, "")
	if len(z.Groups) != 0 {
		t.Fatalf("groups = %+v, dominio público no debe entrar en zonas locales", z.Groups)
	}
}
