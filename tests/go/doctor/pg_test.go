package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

// pgEnv arma entorno con pg_mode y, opcionalmente, un bindir falso con
// los 5 binarios (initdb, pg_ctl, psql, pg_dump, pg_restore).
func pgEnv(t *testing.T, mode string, withBins bool) Env {
	t.Helper()
	e := testEnv()
	e.GetSetting = func(key string) string {
		if key == "port_pool" {
			return "3000-3002"
		}
		if key == "pg_mode" {
			return mode
		}
		if key == "pg_bin" && withBins {
			return ""
		}
		return ""
	}
	if withBins {
		dir := t.TempDir()
		suf := ""
		if filepath.Separator == '\\' {
			suf = ".exe"
		}
		for _, n := range []string{"initdb", "pg_ctl", "psql", "pg_dump", "pg_restore"} {
			if err := os.WriteFile(filepath.Join(dir, n+suf), []byte("x"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		e.GetSetting = func(key string) string {
			switch key {
			case "port_pool":
				return "3000-3002"
			case "pg_mode":
				return mode
			case "pg_bin":
				return filepath.Join(dir, "pg_ctl"+suf)
			}
			return ""
		}
	}
	return e
}

func TestPgDesactivadoNoChequea(t *testing.T) {
	for _, mode := range []string{"", "off", "otro"} {
		cs := Run(pgEnv(t, mode, false))
		if got := find(cs, "postgres"); got != (Check{}) {
			t.Fatalf("mode=%q no debe chequear postgres: %+v", mode, got)
		}
	}
}

func TestPgLocalSinBinsFalla(t *testing.T) {
	e := pgEnv(t, "local", false)
	// pg_bin a un fichero suelto: los hermanos no existen sin tocar PATH.
	lone := filepath.Join(t.TempDir(), "pg_ctl")
	if err := os.WriteFile(lone, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.GetSetting = func(key string) string {
		switch key {
		case "port_pool":
			return "3000-3002"
		case "pg_mode":
			return "local"
		case "pg_bin":
			return lone
		}
		return ""
	}
	cs := Run(e)
	got := find(cs, "postgres")
	if got.State != Fail || got.Hint == "" {
		t.Fatalf("sin bins debe bloquear con hint: %+v", got)
	}
}

func TestPgLocalConBinsPasa(t *testing.T) {
	cs := Run(pgEnv(t, "local", true))
	got := find(cs, "postgres")
	if got.State != OK {
		t.Fatalf("con bins debe pasar: %+v", got)
	}
}
