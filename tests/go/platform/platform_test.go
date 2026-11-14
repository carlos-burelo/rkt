package platform

import (
	"strings"
	"testing"
)

func TestDefaultsNoVacios(t *testing.T) {
	if DefaultAppsDir() == "" || DefaultCertsDir() == "" {
		t.Fatal("defaults vacíos")
	}
}

// Con home, todo cuelga de él; sin home, el default por plataforma.
func TestDirsBajoHome(t *testing.T) {
	if got := AppsDir(""); got != DefaultAppsDir() {
		t.Fatalf("AppsDir vacío = %q", got)
	}
	if got := CertsDir(""); got != DefaultCertsDir() {
		t.Fatalf("CertsDir vacío = %q", got)
	}
	home := t.TempDir()
	if got := AppsDir(home); !strings.HasSuffix(got, "apps") || !strings.HasPrefix(got, home) {
		t.Fatalf("AppsDir = %q", got)
	}
	if got := CertsDir(home); !strings.HasSuffix(got, "certs") || !strings.HasPrefix(got, home) {
		t.Fatalf("CertsDir = %q", got)
	}
}

func TestShellEjecuta(t *testing.T) {
	out, err := Shell("echo rkt").CombinedOutput()
	if err != nil {
		t.Fatalf("shell: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "rkt") {
		t.Fatalf("salida inesperada: %q", out)
	}
}
