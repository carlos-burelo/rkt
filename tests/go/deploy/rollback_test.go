package deploy

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Rollback blue-green: el slot inactivo es la versión anterior. Sin tests
// este camino (el que salva un deploy malo) no estaba corroborado.
func TestRollbackHappy(t *testing.T) {
	gitOK(t)
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	prev := filepath.Join(base, "live.b")
	os.MkdirAll(live, 0o755)
	os.MkdirAll(prev, 0o755)
	os.WriteFile(filepath.Join(prev, "old.txt"), []byte("viejo\n"), 0o644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{}
	defer ctl.close()
	app := testApp(live, "https://example.com/o/r.git", nil)
	if err := Rollback(app, ctl, log); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if ctl.cutoverDir != prev {
		t.Fatalf("corte a %q, quiere %q", ctl.cutoverDir, prev)
	}
	if ctl.cutoverPort == "" {
		t.Fatal("corte sin puerto")
	}
	// el slot activo queda intacto hasta el corte
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("slot activo intacto: %v", err)
	}
}

func TestRollbackSinSlotPrevio(t *testing.T) {
	live := filepath.Join(t.TempDir(), "live.a")
	os.MkdirAll(live, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{}
	defer ctl.close()
	app := testApp(live, "https://example.com/o/r.git", nil)
	err := Rollback(app, ctl, log)
	if err == nil || !strings.Contains(err.Error(), "sin versión previa") {
		t.Fatalf("debió fallar por falta de slot: %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si no hay versión previa")
	}
}

func TestRollbackCanarioMudo(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	prev := filepath.Join(base, "live.b")
	os.MkdirAll(live, 0o755)
	os.MkdirAll(prev, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{noHTTP: true}
	defer ctl.close()
	app := testApp(live, "https://example.com/o/r.git", nil)
	err := Rollback(app, ctl, log)
	if err == nil || !strings.Contains(err.Error(), "canario") {
		t.Fatalf("debió fallar por canario mudo: %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el canario no responde")
	}
}
