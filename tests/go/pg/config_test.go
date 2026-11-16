package pg

import (
	"strings"
	"testing"
)

func testGet(set map[string]string) func(string) string {
	return func(k string) string { return set[k] }
}

func TestLoadConfigApagado(t *testing.T) {
	for _, mode := range []string{"", "off", "otro"} {
		if _, err := LoadConfig(testGet(map[string]string{"pg_mode": mode})); err == nil {
			t.Fatalf("mode %q debe fallar o pedir local", mode)
		}
	}
}

func TestLoadConfigLocal(t *testing.T) {
	cfg, err := LoadConfig(testGet(map[string]string{
		"pg_mode": "local", "pg_port": "9999", "pg_database": "rkt",
	}))
	if err != nil {
		if !strings.Contains(err.Error(), "psql") {
			t.Fatalf("formato válido no debe fallar por eso: %v", err)
		}
	} else {
		if cfg.Port != 9999 || cfg.DB != "rkt" || cfg.User != "postgres" {
			t.Fatalf("defaults: %+v", cfg)
		}
	}
	if _, err := LoadConfig(testGet(map[string]string{"pg_mode": "local", "pg_port": "x"})); err == nil {
		t.Fatal("puerto inválido debe fallar")
	}
	if _, err := LoadConfig(testGet(map[string]string{"pg_mode": "local", "pg_database": "MAL"})); err == nil {
		t.Fatal("base inválida debe fallar")
	}
}
