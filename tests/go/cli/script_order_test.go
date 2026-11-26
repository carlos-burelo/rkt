package cli

// Orden web-antes-que-Go: go:embed exige web/dist con ficheros reales;
// vet, build y test fallan en checkout limpio si el dashboard no se
// compiló antes (main rojo por esto: el job corría vet antes que
// `pnpm --dir web build`). Se fija el orden en los 3 hogares.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebAntesDeGo(t *testing.T) {
	r, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ archivo, antes, despues string }{
		{".github/workflows/ci.yml", "pnpm install --frozen-lockfile", "go build"},
		{".github/workflows/release.yml", "pnpm install --frozen-lockfile", "go build"},
		{"scripts/strict.ps1", "pnpm install --frozen-lockfile", "run.go vet"},
		{"scripts/gate.ps1", "pnpm --dir web build", "go build ./..."},
	} {
		b, err := os.ReadFile(filepath.Join(r, filepath.FromSlash(tc.archivo)))
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)
		iAntes, iDespues := strings.Index(body, tc.antes), strings.Index(body, tc.despues)
		if iAntes < 0 {
			t.Errorf("%s sin paso %q", tc.archivo, tc.antes)
		}
		if iDespues < 0 {
			t.Errorf("%s sin paso %q", tc.archivo, tc.despues)
		}
		if iAntes >= 0 && iDespues >= 0 && iAntes > iDespues {
			t.Errorf("%s: %q debe ir antes que %q (embed exige dist)", tc.archivo, tc.antes, tc.despues)
		}
	}
}
