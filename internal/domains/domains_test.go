package domains

import (
	"errors"
	"testing"
)

func TestDefaultSegunModo(t *testing.T) {
	if got, err := Default("tienda", "ejemplo.com", ""); err != nil || got != "tienda.localhost" {
		t.Fatalf("local = %q %v", got, err)
	}
	if got, err := Default("tienda", "ejemplo.com", "192.168.1.10"); err != nil || got != "tienda.localhost" {
		t.Fatalf("ip privada = %q %v", got, err)
	}
	if got, err := Default("tienda", "ejemplo.com", "203.0.113.7"); err != nil || got != "tienda.ejemplo.com" {
		t.Fatalf("público = %q %v", got, err)
	}
	if _, err := Default("tienda", "", "203.0.113.7"); !errors.Is(err, ErrDomainRequired) {
		t.Fatalf("sin base = %v, quiero ErrDomainRequired", err)
	}
	if _, err := Default("tienda", "localhost", "203.0.113.7"); !errors.Is(err, ErrDomainRequired) {
		t.Fatalf("base local = %v, quiero ErrDomainRequired", err)
	}
	if _, err := Default("", "ejemplo.com", ""); err == nil {
		t.Fatal("nombre vacío debía fallar")
	}
}

func TestSuggestionsLocal(t *testing.T) {
	got := Suggestions("coatl-in", "", "", nil)
	if len(got) != 2 || got[0].Host != "coatl-in.localhost" || got[1].Host != "coatl-in.test" {
		t.Fatalf("local = %+v", got)
	}
	used := map[string]bool{"coatl-in.localhost": true}
	got = Suggestions("coatl-in", "ejemplo.com", "", used)
	if len(got) != 2 || got[0].Host != "coatl-in.test" || got[1].Host != "coatl-in.ejemplo.com" {
		t.Fatalf("local con base = %+v", got)
	}
}

func TestSuggestionsPublic(t *testing.T) {
	got := Suggestions("coatl-in", "ejemplo.com", "203.0.113.7", nil)
	if len(got) != 1 || got[0].Host != "coatl-in.ejemplo.com" || got[0].Kind != KindBase {
		t.Fatalf("público = %+v", got)
	}
	if got := Suggestions("coatl-in", "", "203.0.113.7", nil); len(got) != 0 {
		t.Fatalf("público sin base = %+v, quiero ninguna", got)
	}
	if !RequiresDomain("203.0.113.7", "") || RequiresDomain("", "") || RequiresDomain("203.0.113.7", "ejemplo.com") {
		t.Fatal("RequiresDomain mal")
	}
}

func TestPublic(t *testing.T) {
	for ip, want := range map[string]bool{
		"":             false,
		"127.0.0.1":    false,
		"10.0.0.2":     false,
		"192.168.1.10": false,
		"203.0.113.7":  true,
		"2001:db8::1":  true,
		"fe80::1":      false,
		"no-es-una-ip": false,
	} {
		if got := Public(ip); got != want {
			t.Fatalf("Public(%q) = %v, quiero %v", ip, got, want)
		}
	}
}
