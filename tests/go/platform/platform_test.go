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

func TestShellEjecuta(t *testing.T) {
	out, err := Shell("echo rkt").CombinedOutput()
	if err != nil {
		t.Fatalf("shell: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "rkt") {
		t.Fatalf("salida inesperada: %q", out)
	}
}
