package config

import (
	"os"
	"testing"
)

func testConfig(t *testing.T, content string) *Config {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestMatchExact(t *testing.T) {
	cfg := testConfig(t, `
listen_http: ":80"
routes:
  - hosts: ["app.ejemplo.com"]
    target: "http://127.0.0.1:3000"
`)
	if cfg.Match("app.ejemplo.com") == nil {
		t.Fatal("debió matchear exacto")
	}
	if cfg.Match("app.ejemplo.com:80") == nil {
		t.Fatal("debió matchear con puerto")
	}
	if cfg.Match("otro.ejemplo.com") != nil {
		t.Fatal("no debió matchear")
	}
}

func TestMatchWildcard(t *testing.T) {
	cfg := testConfig(t, `
routes:
  - hosts: ["app.ejemplo.com"]
    target: "http://127.0.0.1:3000"
  - hosts: ["*.ejemplo.com"]
    target: "http://127.0.0.1:4000"
`)
	if u := cfg.Match("app.ejemplo.com"); u == nil || u.Host != "127.0.0.1:3000" {
		t.Fatalf("exacto debe ganar, got %v", u)
	}
	if u := cfg.Match("otro.ejemplo.com"); u == nil || u.Host != "127.0.0.1:4000" {
		t.Fatalf("wildcard debe matchear, got %v", u)
	}
	if cfg.Match("ejemplo.com") != nil {
		t.Fatal("base no debe matchear *.ejemplo.com")
	}
}

func TestMatchCaseInsensitive(t *testing.T) {
	cfg := testConfig(t, `
routes:
  - hosts: ["App.Ejemplo.COM"]
    target: "http://127.0.0.1:3000"
`)
	if cfg.Match("app.ejemplo.com") == nil {
		t.Fatal("match debe ser case-insensitive")
	}
}

func TestDuplicateHostRejected(t *testing.T) {
	f, _ := os.CreateTemp("", "rkt-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(`
routes:
  - hosts: ["a.com"]
    target: "http://127.0.0.1:3000"
  - hosts: ["a.com"]
    target: "http://127.0.0.1:4000"
`)
	f.Close()
	if _, err := Load(f.Name()); err == nil {
		t.Fatal("duplicado debe fallar")
	}
}
