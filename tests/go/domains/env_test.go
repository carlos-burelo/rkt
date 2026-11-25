package domains

import "testing"

func TestClassify(t *testing.T) {
	const ip = "148.236.115.31"
	cases := []struct {
		name, base, pub string
		a               []string
		kind            string
	}{
		{"local sin ip", "", "", nil, EnvLocal},
		{"local ip privada", "", "192.168.1.5", nil, EnvLocal},
		{"ip publica sin dominio", "", ip, nil, EnvPublicIP},
		{"ip publica dominio local", "app.localhost", ip, nil, EnvPublicIP},
		{"base es ip", ip, ip, nil, EnvBadDomain},
		{"base es ip sin publica", "10.0.0.1", "", nil, EnvBadDomain},
		{"sin registro A", "x.dev", ip, nil, EnvBadDomain},
		{"A distinto", "x.dev", ip, []string{"1.2.3.4"}, EnvBadDomain},
		{"prod", "x.dev", ip, []string{"1.2.3.4", ip}, EnvProd},
		{"dominio sin ip publica", "x.dev", "", nil, EnvLocal},
	}
	for _, c := range cases {
		e := Classify(c.base, c.pub, c.a)
		if e.Kind != c.kind {
			t.Errorf("%s: kind=%q quiero %q", c.name, e.Kind, c.kind)
		}
		if e.Blocked() && (e.Problem == "" || e.Fix == "") {
			t.Errorf("%s: bloqueado sin problema/fix", c.name)
		}
	}
}
