package geoip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `"1.0.0.0","1.0.0.255","AU"
"8.8.8.0","8.8.8.255","US"
"200.0.0.0","200.0.255.255","MX"
"2001:db8::","2001:db8:ffff:ffff:ffff:ffff:ffff:ffff","ZZ"
basura
"x","y","US"
`

func TestCountry(t *testing.T) {
	p := filepath.Join(t.TempDir(), "geoip.csv")
	l := Open(p)
	if l.Ready() || l.Country("8.8.8.8") != "" {
		t.Fatal("sin archivo no debe resolver")
	}
	if err := install(strings.NewReader(sample), p); err != nil {
		t.Fatal(err)
	}
	l = Open(p)
	if !l.Ready() {
		t.Fatal("base no cargó")
	}
	cases := map[string]string{
		"8.8.8.8":        "US",
		"1.0.0.0":        "AU",
		"1.0.0.255":      "AU",
		"1.0.1.0":        "",
		"200.0.12.1":     "MX",
		"::ffff:8.8.8.1": "US", // IPv4 mapeada
		"2001:db8::1":    "ZZ",
		"2001:db9::1":    "",
		"no-es-ip":       "",
		"0.0.0.1":        "",
	}
	for ip, want := range cases {
		if got := l.Country(ip); got != want {
			t.Errorf("%s = %q quiero %q", ip, got, want)
		}
	}
}

func TestInstallRechazaVacio(t *testing.T) {
	p := filepath.Join(t.TempDir(), "geoip.csv")
	if err := install(strings.NewReader("nada,util\n"), p); err == nil {
		t.Fatal("CSV sin rangos debe fallar")
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("no debe publicar un archivo inválido")
	}
}
