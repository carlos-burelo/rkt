package appview

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
	z := BuildZones(routes, "")
	if len(z.Groups) != 2 {
		t.Fatalf("grupos = %+v, quiero 2", z.Groups)
	}
	if z.Groups[0].Base != "api.test" || z.Groups[1].Base != "coatl-in.localhost" {
		t.Fatalf("bases = %q / %q", z.Groups[0].Base, z.Groups[1].Base)
	}
	g := z.Groups[1]
	if g.Target != "http://127.0.0.1:3001" {
		t.Fatalf("target base = %q", g.Target)
	}
	if len(g.Hosts) != 2 || !g.Hosts[0].IsBase || g.Hosts[1].Host != "admin.coatl-in.localhost" {
		t.Fatalf("hosts = %+v", g.Hosts)
	}
}

func TestBuildZonesBaseDomainComoZona(t *testing.T) {
	z := BuildZones([]config.Route{{Hosts: []string{"app.midominio.dev"}, Target: "http://127.0.0.1:3000"}}, "midominio.dev")
	found := false
	for _, s := range z.Suffixes {
		if s == "midominio.dev" {
			found = true
		}
	}
	if !found {
		t.Fatalf("suffixes = %v", z.Suffixes)
	}
	if len(z.Groups) != 1 || z.Groups[0].Base != "midominio.dev" {
		t.Fatalf("groups = %+v", z.Groups)
	}
}

func TestBuildZonesIgnoraPublicos(t *testing.T) {
	z := BuildZones([]config.Route{{Hosts: []string{"app.ejemplo.com"}, Target: "http://127.0.0.1:3000"}}, "")
	if len(z.Groups) != 0 {
		t.Fatalf("groups = %+v", z.Groups)
	}
}

func TestAppDescribeDominiosYDNS(t *testing.T) {
	cfg := &config.Config{
		Apps: []config.App{{Name: "a", Env: map[string]string{"PORT": "3001"}, Args: []string{"__static", "/dist"}}},
		Routes: []config.Route{
			{Hosts: []string{"a.localhost"}, Target: "http://127.0.0.1:3001"},
			{Hosts: []string{"otra.test"}, Target: "http://127.0.0.1:9999"},
		},
	}
	m := App(cfg, cfg.Apps[0])
	if m["port"] != "3001" || m["static"] != "/dist" {
		t.Fatalf("vista = %+v", m)
	}
	domains := m["domains"].([]string)
	if len(domains) != 1 || domains[0] != "a.localhost" {
		t.Fatalf("domains = %v", domains)
	}
	if m["dns_ok"] != true {
		t.Fatalf("dns_ok = %v, localhost resuelve", m["dns_ok"])
	}
}

func TestAppsYResolvesLocal(t *testing.T) {
	cfg := &config.Config{
		Apps: []config.App{
			{Name: "a", Env: map[string]string{"PORT": "3001"}},
			{Name: "b", Args: []string{"__static", `C:\sitio`}, Env: map[string]string{"PORT": "3002"}},
			{Name: "c"},
		},
		Routes: []config.Route{
			{Hosts: []string{"a.localhost"}, Target: "http://127.0.0.1:3001"},
			{Hosts: []string{"b.localhost"}, Target: "http://127.0.0.1:3002"},
		},
	}
	all := Apps(cfg)
	if len(all) != 3 || all[1]["static"] != `C:\sitio` {
		t.Fatalf("Apps = %+v", all)
	}
	if d := Domains(cfg, cfg.Apps[2]); len(d) != 0 {
		t.Fatalf("sin PORT no hay dominios: %v", d)
	}
	if !ResolvesLocal("localhost") || !ResolvesLocal("app.localhost") {
		t.Fatal("localhost debe resolver local")
	}
	if !anyLoopback([]string{"10.0.0.1", "::1"}) || !anyLoopback([]string{"127.0.0.1"}) {
		t.Fatal("anyLoopback debe reconocer loopback")
	}
	if anyLoopback([]string{"10.0.0.1", "8.8.8.8"}) {
		t.Fatal("IPs públicas no son loopback")
	}
	if lessHost(ZoneHost{Host: "z", IsBase: false}, ZoneHost{Host: "a", IsBase: true}) {
		t.Fatal("apex debe ir primero")
	}
	if !lessHost(ZoneHost{Host: "a"}, ZoneHost{Host: "b"}) {
		t.Fatal("mismo nivel ordena por host")
	}
	if !ResolvesLocal("127.0.0.1") {
		t.Fatal("127.0.0.1 debe resolver local")
	}
	if ResolvesLocal("dominio-que-no-existe-rkt.invalid") {
		t.Fatal("dominio inexistente no debe resolver local")
	}
	// dns_ok false cuando el host no resuelve a loopback
	cfg.Routes = append(cfg.Routes, config.Route{Hosts: []string{"x.invalid"}, Target: "http://127.0.0.1:3002"})
	if m := App(cfg, cfg.Apps[1]); m["dns_ok"] != false {
		t.Fatalf("dns_ok = %v, quiere false", m["dns_ok"])
	}
	// deploy se describe cuando existe
	cfg.Apps[0].Deploy = &config.Deploy{Repo: "r", Ref: "main", Build: []string{"x"}, Auto: true}
	m := App(cfg, cfg.Apps[0])
	if m["deploy"] == nil {
		t.Fatal("deploy debe aparecer")
	}
}
