package cfgfile

import (
	"testing"

	"rkt-proxy/internal/config"
)

func fleetWithApp(port string) *config.Config {
	return &config.Config{
		Apps: []config.App{{
			Name: "coatlin", Command: "node", Dir: "/x",
			Env: map[string]string{"PORT": port},
		}},
		Routes: []config.Route{
			{Hosts: []string{"viejo.localhost"}, Target: "http://127.0.0.1:" + port},
			{Hosts: []string{"otra.test"}, Target: "http://127.0.0.1:9999"},
		},
	}
}

func TestSetAppDomainReemplazaHostsDelPuerto(t *testing.T) {
	cp := fleetWithApp("3001")
	if err := SetAppDomain(cp, "coatlin", "https://coatl-in.localhost/"); err != nil {
		t.Fatal(err)
	}
	var own, other int
	for _, r := range cp.Routes {
		switch r.Target {
		case "http://127.0.0.1:3001":
			own++
			if len(r.Hosts) != 1 || r.Hosts[0] != "coatl-in.localhost" {
				t.Fatalf("ruta propia = %+v, quiero el host nuevo normalizado", r)
			}
		default:
			other++
		}
	}
	if own != 1 || other != 1 {
		t.Fatalf("rutas = %+v, quiero 1 propia y respetar la ajena", cp.Routes)
	}
}

func TestSetAppDomainVacioQuitaDominio(t *testing.T) {
	cp := fleetWithApp("3001")
	if err := SetAppDomain(cp, "coatlin", ""); err != nil {
		t.Fatal(err)
	}
	if len(cp.Routes) != 1 || cp.Routes[0].Hosts[0] != "otra.test" {
		t.Fatalf("rutas = %+v, quiero solo la ajena", cp.Routes)
	}
}

func TestSetAppPortRetargetaRutas(t *testing.T) {
	cp := fleetWithApp("3001")
	if err := SetAppPort(cp, "coatlin", 4100); err != nil {
		t.Fatal(err)
	}
	if got := cp.Apps[0].Env["PORT"]; got != "4100" {
		t.Fatalf("PORT = %q", got)
	}
	if cp.Routes[0].Target != "http://127.0.0.1:4100" {
		t.Fatalf("target = %q, quiero el puerto nuevo", cp.Routes[0].Target)
	}
	if cp.Routes[1].Target != "http://127.0.0.1:9999" {
		t.Fatalf("ruta ajena tocada: %q", cp.Routes[1].Target)
	}
}
