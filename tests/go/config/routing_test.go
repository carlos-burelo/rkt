package config

// Las dos estrategias de enrutado: dominio y prefijo de ruta.

import (
	"testing"
)

func mustResolve(t *testing.T, r *Route) {
	t.Helper()
	if err := r.Resolve(); err != nil {
		t.Fatalf("ruta %+v: %v", r, err)
	}
}

func routesVivas(t *testing.T, rr []Route) *Config {
	t.Helper()
	for i := range rr {
		mustResolve(t, &rr[i])
	}
	return &Config{Routes: rr}
}

func TestMatchRouteTabla(t *testing.T) {
	cfg := routesVivas(t, []Route{
		{Hosts: []string{"app.midominio.com"}, Target: "http://127.0.0.1:1001"},
		{Hosts: []string{"*.midominio.com"}, Target: "http://127.0.0.1:1002"},
		{Path: "/app", Target: "http://127.0.0.1:1003"},
		{Hosts: []string{"otro.com"}, Path: "/app", Target: "http://127.0.0.1:1004"},
	})
	cases := []struct {
		host, path string
		want       string
		strip      string
	}{
		// host-based
		{"app.midominio.com", "/", "http://127.0.0.1:1001", ""},
		{"APP.MIDOMINIO.COM:443", "/x", "http://127.0.0.1:1001", ""},
		{"sub.midominio.com", "/", "http://127.0.0.1:1002", ""},
		// path-based sobre IP (sin hosts: vale cualquier host)
		{"148.236.150.38", "/app", "http://127.0.0.1:1003", "/app"},
		{"148.236.150.38", "/app/inicio", "http://127.0.0.1:1003", "/app"},
		{"148.236.150.38", "/other", "", ""},
		{"148.236.150.38", "/app2", "", ""}, // prefijo parcial no vale
		{"cualquiera.test", "/app/y", "http://127.0.0.1:1003", "/app"},
		// host + path gana al path suelto
		{"otro.com", "/app/y", "http://127.0.0.1:1004", "/app"},
		{"otro.com", "/", "", ""},
		{"noexiste.test", "/nada", "", ""},
	}
	for _, c := range cases {
		m := cfg.MatchRoute(c.host, c.path)
		var got, strip string
		if m != nil {
			got, strip = m.Target.String(), m.Strip
		}
		if got != c.want || strip != c.strip {
			t.Errorf("MatchRoute(%q, %q) = %q/%q, quiere %q/%q",
				c.host, c.path, got, strip, c.want, c.strip)
		}
	}
}

func TestMatchRoutePrefijoLargoGana(t *testing.T) {
	cfg := routesVivas(t, []Route{
		{Path: "/app", Target: "http://127.0.0.1:1"},
		{Path: "/app/admin", Target: "http://127.0.0.1:2"},
		{Hosts: []string{"h.test"}, Path: "/app", Target: "http://127.0.0.1:3"},
		{Hosts: []string{"*.test"}, Path: "/app", Target: "http://127.0.0.1:4"},
	})
	if m := cfg.MatchRoute("x.test", "/app/admin/panel"); m == nil || m.Target.String() != "http://127.0.0.1:2" {
		t.Fatalf("prefijo largo: %+v", m)
	}
	// Mismo path: exacto > wildcard > cualquiera.
	if m := cfg.MatchRoute("h.test", "/app/y"); m == nil || m.Target.String() != "http://127.0.0.1:3" {
		t.Fatalf("host exacto: %+v", m)
	}
	if m := cfg.MatchRoute("z.test", "/app/y"); m == nil || m.Target.String() != "http://127.0.0.1:4" {
		t.Fatalf("wildcard: %+v", m)
	}
}

func TestMatchCompatSinCambios(t *testing.T) {
	cfg := routesVivas(t, []Route{
		{Hosts: []string{"a.test", "*.b.test"}, Target: "http://127.0.0.1:3000"},
	})
	for _, h := range []string{"a.test", "A.TEST:443", "x.b.test"} {
		if cfg.Match(h) == nil {
			t.Fatalf("Match(%q) = nil", h)
		}
	}
	for _, h := range []string{"", "otro.test", "b.test"} {
		if cfg.Match(h) != nil {
			t.Fatalf("Match(%q) debió ser nil", h)
		}
	}
}

func TestValidateRutas(t *testing.T) {
	ok := []Route{
		{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:1"},
		{Path: "/app", Target: "http://127.0.0.1:2"},
		{Hosts: []string{"a.test"}, Path: "/app", Target: "http://127.0.0.1:4"},
		{Hosts: []string{"*.w.test"}, Path: "/app", Target: "http://127.0.0.1:6"},
	}
	if err := ValidateRoutes(ok); err != nil {
		t.Fatalf("válidas: %v", err)
	}
	rs := []Route{{Hosts: []string{"  A.Test "}, Path: "/app/", Target: "http://127.0.0.1:1"}}
	if err := ValidateRoutes(rs); err != nil {
		t.Fatal(err)
	}
	if rs[0].Hosts[0] != "a.test" || rs[0].Path != "/app" {
		t.Fatalf("sin normalizar: %+v", rs[0])
	}

	bad := []struct {
		name string
		r    []Route
	}{
		{"sin selector", []Route{{Target: "http://127.0.0.1:1"}}},
		{"sin target", []Route{{Hosts: []string{"a.test"}}}},
		{"path sin barra", []Route{{Path: "app", Target: "http://127.0.0.1:1"}}},
		{"host vacío", []Route{{Hosts: []string{"  "}, Target: "http://127.0.0.1:1"}}},
		{"duplicado exacto", []Route{
			{Hosts: []string{"a.test"}, Path: "/app", Target: "http://127.0.0.1:1"},
			{Hosts: []string{"A.TEST"}, Path: "/app", Target: "http://127.0.0.1:2"},
		}},
		{"duplicado path suelto", []Route{
			{Path: "/app", Target: "http://127.0.0.1:1"},
			{Path: "/app/", Target: "http://127.0.0.1:2"},
		}},
	}
	for _, c := range bad {
		if err := ValidateRoutes(c.r); err == nil {
			t.Fatalf("%s: debió fallar", c.name)
		}
	}
}
