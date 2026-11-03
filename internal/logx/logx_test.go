package logx

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRutasCanonicas: db, log operativo y access log viven bajo logs/.
func TestRutasCanonicas(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.dev.yaml")
	dir := DirFor(cfg)
	if filepath.Base(dir) != "logs" {
		t.Fatalf("DirFor = %q", dir)
	}
	cases := map[string]string{
		"db":     DBPath(cfg),
		"log":    Path(cfg),
		"access": AccessPath(cfg),
	}
	for name, got := range cases {
		if filepath.Dir(got) != dir {
			t.Fatalf("%s fuera de logs: %q", name, got)
		}
	}
	if DBPath(cfg) != filepath.Join(dir, "rkt.db") {
		t.Fatalf("db = %q", DBPath(cfg))
	}
	if !strings.HasSuffix(AccessPath(cfg), "access.log") {
		t.Fatalf("access = %q", AccessPath(cfg))
	}
}
