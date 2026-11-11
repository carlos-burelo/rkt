package config

// Las tres estrategias de enrutado: dominio, prefijo de ruta y puerto.

import (
	"testing"
)

func mustResolve(t *testing.T, r *Route) {
	t.Helper()
	if err := r.Resolve(); err != nil {
		t.Fatalf("resolve %+v: %v", r, err)
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
		{Port: 3000, Target: "http://127.0.0.1:1005"},
		{Hosts: []string{"pub.test"}, Port: 3001, Target: "http://127.0.0.1:1006"},
	})
	cases := []struct {
		host, path string
		port       int
		want       string
		strip      string
	}{
		// host-based
		{"app.midominio.com", "/", 0, "http://127.0.0.1:1001", ""},
		{"APP.MIDOMINIO.COM:80", "/x", 0, "http://127.0.0.1:1001", ""},
		{"sub.midominio.com", "/", 0, "http://127.0.0.1:1002", ""},
		// path-based sobre IP (sin hosts): la IP entra como host cualquiera
		{"148.236.150.38", "/app", 0, "http://127.0.0.1:1003", "/app"},
		{"148.236.150.38", "/app/inicio", 0, "http://127.0.0.1:1003", "/app"},
		{"148.236.150.38", "/other", 0, "", ""},
		{"148.236.150.38", "/app2", 0, "", ""}, // prefijo parcial no vale
		{"cualquiera.test", "/app/y", 0, "http://127.0.0.1:1003", "/app"},
		// host + path gana al path suelto
		{"otro.com", "/app/y", 0, "http://127.0.0.1:1004", "/app"},
		{"otro.com", "/", 0, "", ""},
		// port-based: el puerto manda sobre el host
		{"148.236.150.38", "/", 3000, "http://127.0.0.1:1005", ""},
		{"cualquiera.test", "/", 3000, "http://127.0.0.1:1005", ""},
		{"148.236.150.38", "/app", 3000, "http://127.0.0.1:1005", ""}, // puerto antes que path
		{"pub.test", "/", 3001, "http://127.0.0.1:1006", ""},
		{"otra.test", "/", 3001, "", ""}, // puerto+host que no coincide
		// En un puerto extra manda el puerto: el listener existe por las
		// rutas de puerto y no filtra hosts principales por Host.
		{"app.midominio.com", "/", 3000, "http://127.0.0.1:1005", ""},
		// En el principal las rutas de puerto no existen...
		{"pub.test", "/", 0, "", ""},
	}
	for _, c := range cases {
		m := cfg.MatchRoute(c.host, c.port, c.path)
		var got, strip string
		if m != nil {
			got, strip = m.Target.String(), m.Strip
		}
		if got != c.want || strip != c.strip {
			t.Errorf("MatchRoute(%q,%d,%q) = %q/%q, quiere %q/%q",
				c.host, c.port, c.path, got, strip, c.want, c.strip)
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
	if m := cfg.MatchRoute("x.test", 0, "/app/admin/panel"); m == nil || m.Target.String() != "http://127.0.0.1:2" {
		t.Fatalf("prefijo largo: %+v", m)
	}
	// Mismo path: exacto > wildcard > cualquiera.
	if m := cfg.MatchRoute("h.test", 0, "/app/y"); m == nil || m.Target.String() != "http://127.0.0.1:3" {
		t.Fatalf("host exacto: %+v", m)
	}
	if m := cfg.MatchRoute("z.test", 0, "/app/y"); m == nil || m.Target.String() != "http://127.0.0.1:4" {
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

func TestValidateRoutesEstrategias(t *testing.T) {
	ok := []Route{
		{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:1"},
		{Path: "/app", Target: "http://127.0.0.1:2"},
		{Port: 3000, Target: "http://127.0.0.1:3"},
		{Hosts: []string{"a.test"}, Path: "/app", Target: "http://127.0.0.1:4"},
		{Hosts: []string{"a.test"}, Port: 3001, Target: "http://127.0.0.1:5"},
		{Hosts: []string{"*.w.test"}, Path: "/app", Target: "http://127.0.0.1:6"},
	}
	if err := ValidateRoutes(ok); err != nil {
		t.Fatalf("válidas: %v", err)
	}
	// Normaliza: mayúsculas/espacios fuera, trailing slash fuera.
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
		{"port 0 explícito sin nada más", []Route{{Port: 0, Target: "http://127.0.0.1:1"}}},
		{"port fuera de rango", []Route{{Port: 99999, Target: "http://127.0.0.1:1"}}},
		{"host vacío", []Route{{Hosts: []string{"  "}, Target: "http://127.0.0.1:1"}}},
		{"duplicado exacto", []Route{
			{Hosts: []string{"a.test"}, Path: "/app", Target: "http://127.0.0.1:1"},
			{Hosts: []string{"A.TEST"}, Path: "/app", Target: "http://127.0.0.1:2"},
		}},
		{"duplicado path suelto", []Route{
			{Path: "/app", Target: "http://127.0.0.1:1"},
			{Path: "/app/", Target: "http://127.0.0.1:2"},
		}},
		{"duplicado puerto", []Route{
			{Port: 3000, Target: "http://127.0.0.1:1"},
			{Port: 3000, Target: "http://127.0.0.1:2"},
		}},
	}
	for _, c := range bad {
		if err := ValidateRoutes(c.r); err == nil {
			t.Fatalf("%s: debió fallar", c.name)
		}
	}
}

func TestValidatePuertosReservados(t *testing.T) {
	base := func() *Config {
		cfg := Default()
		cfg.ApplyDefaults()
		cfg.ListenHTTP = "0.0.0.0:80"
		cfg.ListenHTTPS = ""
		cfg.Dashboard.Listen = "127.0.0.1:8090"
		cfg.Healthz.Listen = "127.0.0.1:8091"
		cfg.Healthz.Enabled = true
		return cfg
	}
	cfg := base()
	cfg.Routes = []Route{{Port: 3000, Target: "http://127.0.0.1:9"}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("puerto libre: %v", err)
	}
	for _, tc := range []struct {
		name string
		port int
	}{
		{"http", 80},
		{"dashboard", 8090},
		{"healthz", 8091},
	} {
		cfg := base()
		cfg.Routes = []Route{{Port: tc.port, Target: "http://127.0.0.1:9"}}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("puerto %s (%d) debió chocar", tc.name, tc.port)
		}
	}
}

func TestNeedRestartPuertos(t *testing.T) {
	a := &Config{Routes: []Route{{Hosts: []string{"x.test"}, Target: "http://127.0.0.1:1"}}}
	mk := func(ports ...int) *Config {
		rs := []Route{{Hosts: []string{"x.test"}, Target: "http://127.0.0.1:1"}}
		for _, p := range ports {
			rs = append(rs, Route{Port: p, Target: "http://127.0.0.1:9"})
		}
		return &Config{Routes: rs}
	}
	has := func(out []string) bool {
		for _, s := range out {
			if s == "routes.ports" {
				return true
			}
		}
		return false
	}
	if has(NeedRestart(a, mk())) {
		t.Fatal("sin cambios no debe pedir reinicio")
	}
	if !has(NeedRestart(a, mk(3000))) {
		t.Fatal("puerto nuevo debe pedir reinicio")
	}
	if !has(NeedRestart(mk(3000), mk(3001))) {
		t.Fatal("cambio de puerto debe pedir reinicio")
	}
	if has(NeedRestart(mk(3000, 3001), mk(3001, 3000))) {
		t.Fatal("mismo conjunto en otro orden no debe pedir reinicio")
	}
}
