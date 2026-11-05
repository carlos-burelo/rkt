package dnscheck

import (
	"context"
	"errors"
	"net"
	"testing"
)

type fakeResolver struct {
	ips   []net.IP
	ipErr error
	cname string
	cErr  error
}

func (f fakeResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return f.ips, f.ipErr
}

func (f fakeResolver) LookupCNAME(context.Context, string) (string, error) {
	return f.cname, f.cErr
}

func TestCheckClasificaAAndAAAA(t *testing.T) {
	r := Check(context.Background(), fakeResolver{
		ips:   []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		cname: "destino.test.",
	}, "App.Test.")
	if !r.Resolves || len(r.A) != 1 || r.A[0] != "127.0.0.1" || len(r.AAAA) != 1 || r.AAAA[0] != "::1" {
		t.Fatalf("resultado = %+v", r)
	}
	if r.CNAME != "destino.test" {
		t.Fatalf("cname = %q", r.CNAME)
	}
}

func TestCheckErrorNoFatal(t *testing.T) {
	r := Check(context.Background(), fakeResolver{ipErr: errors.New("nxdomain"), cErr: errors.New("no cname")}, "nope.test")
	if r.Resolves || r.Error == "" || r.CNAME != "" {
		t.Fatalf("resultado = %+v", r)
	}
	if Check(context.Background(), nil, "  ").Error == "" {
		t.Fatal("host vacío debe informar error")
	}
}

func TestExpectedAndMatches(t *testing.T) {
	cases := []struct {
		host, base, ip, want string
	}{
		{"app.localhost", "", "", "127.0.0.1"},
		{"app.test", "", "", "127.0.0.1"},
		{"app.ejemplo.com", "ejemplo.com", "203.0.113.7", "203.0.113.7"},
		{"otro.dev", "ejemplo.com", "", ""},
	}
	for _, c := range cases {
		if got := Expected(c.host, c.base, c.ip); got != c.want {
			t.Fatalf("Expected(%q) = %q, want %q", c.host, got, c.want)
		}
	}
	r := Result{A: []string{"127.0.0.1"}, Resolves: true}
	if !r.Matches("127.0.0.1") || r.Matches("10.0.0.1") {
		t.Fatal("Matches IP")
	}
	if !(Result{Resolves: true}).Matches("") {
		t.Fatal("sin expectativa vale resolverse")
	}
	r6 := Result{AAAA: []string{"2001:db8::1"}, Resolves: true}
	if !r6.Matches("2001:db8::1") {
		t.Fatal("Matches IPv6")
	}
}
