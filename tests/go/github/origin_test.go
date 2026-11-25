package github

import (
	"context"
	"net"
	"testing"

	"rkt/internal/domains"
)

type fakeRes struct{ ips []net.IP }

func (f fakeRes) LookupIP(context.Context, string, string) ([]net.IP, error) { return f.ips, nil }
func (f fakeRes) LookupCNAME(context.Context, string) (string, error)        { return "", nil }

func TestPlanSetup(t *testing.T) {
	const pub = "148.236.115.31"
	// Caso real de prod: IP pública, sin base_domain, :443 por defecto.
	p := PlanSetup("", pub, "", ":443", nil)
	if p.Env.Kind != domains.EnvPublicIP || !p.Env.Blocked() || p.Origin != "https://127.0.0.1" {
		t.Fatalf("prod sin dominio: %+v", p)
	}
	// Local puro: no bloquea.
	if p := PlanSetup("", "", ":8080", "", nil); p.Env.Blocked() || !p.Local {
		t.Fatalf("local: %+v", p)
	}
	// Dominio apuntando a este servidor.
	ok := fakeRes{ips: []net.IP{net.ParseIP(pub)}}
	if p := PlanSetup("x.dev", pub, "", ":443", ok); p.Env.Kind != domains.EnvProd || p.Origin != "https://x.dev" {
		t.Fatalf("prod: %+v", p)
	}
	// Dominio apuntando a otro host.
	bad := fakeRes{ips: []net.IP{net.ParseIP("1.2.3.4")}}
	if p := PlanSetup("x.dev", pub, "", ":443", bad); p.Env.Kind != domains.EnvBadDomain {
		t.Fatalf("dominio-mal: %+v", p)
	}
}
