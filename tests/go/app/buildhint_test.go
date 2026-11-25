package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildHintForLog traduce firmas conocidas a pasos accionables en el log
// que lee el operador (el inglés de pnpm/webpack no le dice qué hacer).
func TestBuildHintForLog(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pnpm := mk("pnpm.log", "? step failed\nError: ERR_PNPM_IGNORED_BUILDS\nhelp: Run \"pnpm approve-builds\"")
	if got := buildHintForLog(pnpm); got == "" {
		t.Fatal("pnpm bloqueado debe dar hint")
	} else if want := "pnpm approve-builds"; !strings.Contains(got, want) {
		t.Fatalf("hint sin comando: %q", got)
	}
	eperm := mk("eperm.log", "EPERM: operation not permitted, symlink 'x' -> 'y'\n[ELIFECYCLE] failed")
	if got := buildHintForLog(eperm); got == "" {
		t.Fatal("EPERM symlink debe dar hint")
	} else if want := "modo desarrollador"; !strings.Contains(got, want) {
		t.Fatalf("hint sin arreglo: %q", got)
	}
	other := mk("otro.log", "SyntaxError: unexpected token\nnpm ERR! code 1")
	if got := buildHintForLog(other); got != "" {
		t.Fatalf("desconocido no debe inventar hint: %q", got)
	}
	if got := buildHintForLog(filepath.Join(dir, "noexiste.log")); got != "" {
		t.Fatalf("sin log no debe dar hint: %q", got)
	}
}
