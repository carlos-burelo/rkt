package domains

import (
	"testing"

	"rkt/internal/dnscheck"
)

func TestBuildGuide(t *testing.T) {
	const ip = "148.236.115.31"
	cases := []struct {
		name, host, base, pub string
		dns                   dnscheck.Result
		status                string
		ready                 bool
		rec                   string
	}{
		{"ok", "app.x.dev", "x.dev", ip, dnscheck.Result{A: []string{ip}, Resolves: true}, GuideOK, true, "app"},
		{"sin registro", "app.x.dev", "x.dev", ip, dnscheck.Result{}, GuideNoRecord, false, "app"},
		{"otra ip", "x.dev", "x.dev", ip, dnscheck.Result{A: []string{"1.1.1.1"}, Resolves: true}, GuideWrongIP, false, "@"},
		{"cname", "app.x.dev", "x.dev", ip, dnscheck.Result{CNAME: "lb.net", A: []string{"2.2.2.2"}, Resolves: true}, GuideCNAME, false, "app"},
		{"ipv6", "a.b.c.dev", "", ip, dnscheck.Result{AAAA: []string{"::2"}, Resolves: true}, GuideIPv6Only, false, "a.b"},
		{"local", "app.localhost", "", ip, dnscheck.Result{}, GuideLocal, true, ""},
		{"sin ip publica", "app.x.dev", "x.dev", "", dnscheck.Result{}, GuideNoPublicIP, false, ""},
	}
	for _, c := range cases {
		g := BuildGuide(c.host, c.base, c.pub, c.dns)
		if g.Status != c.status || g.Ready != c.ready {
			t.Errorf("%s: status=%s ready=%v", c.name, g.Status, g.Ready)
		}
		if c.rec != "" && (g.Record == nil || g.Record.Name != c.rec || g.Record.Value != ip) {
			t.Errorf("%s: record=%+v", c.name, g.Record)
		}
		if len(g.Steps) == 0 || g.Summary == "" {
			t.Errorf("%s: sin pasos o resumen", c.name)
		}
	}
	if g := BuildGuide("app.x.dev", "x.dev", ip, dnscheck.Result{}); g.Wildcard == nil || g.Wildcard.Name != "*" {
		t.Error("falta wildcard bajo base_domain")
	}
}
