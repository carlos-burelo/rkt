package deploy

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Cancelar a mitad del build no debe cortar: el deploy muere y el slot
// activo queda intacto (cero cortes a medias).
func TestDeployCancelMidBuild(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	os.MkdirAll(live, 0o755)

	slow := `i=0; while [ $i -lt 1000000 ]; do i=$((i+1)); done`
	if runtime.GOOS == "windows" {
		// for+dir son builtins de cmd: sin procesos hijos, la kill no
		// deja huérfanos que retengan el dir (ping sí los deja).
		slow = `for /l %i in (1,1,1000000) do @dir C:\Windows\System32 >nul`
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{}
	defer ctl.close()

	// out en archivo como en producción (service abre el log de deploy):
	// con io.Discard Go crea un pipe y Wait esperaría al nieto huérfano.
	out, err := os.CreateTemp(t.TempDir(), "deploy-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := RunContext(ctx, testApp(live, repo, []string{slow}), "", ctl, out, log)
		done <- err
	}()
	time.Sleep(3 * time.Second) // deja que el clone termine y el build arranque
	cancel()

	select {
	case err := <-done:
		t.Logf("deploy cancelado terminó con: %v", err)
		if err == nil {
			t.Fatal("deploy cancelado debió fallar")
		}
	case <-time.After(60 * time.Second):
		t.Fatal("el deploy no respondió a la cancelación")
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte tras cancelar")
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("slot activo intacto: %v", err)
	}
}

// Sin cancelación el contexto manda: error nil solo si todo fue bien.
func TestDeployContextoSinCancelar(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	os.MkdirAll(live, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{}
	defer ctl.close()
	rev, err := RunContext(context.Background(), testApp(live, repo, nil), "", ctl, io.Discard, log)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if rev == "" || rev == "?" {
		t.Fatalf("rev vacío: %q", rev)
	}
	if !strings.HasSuffix(ctl.cutoverDir, "live.b") {
		t.Fatalf("corte a %q", ctl.cutoverDir)
	}
}

// Si el canario no arranca, no hay corte y el slot inactivo se limpia.
func TestDeployCanarioNoArranca(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	os.MkdirAll(live, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{failCanary: true}
	defer ctl.close()
	_, err := RunContext(context.Background(), testApp(live, repo, nil), "", ctl, io.Discard, log)
	if err == nil || !strings.Contains(err.Error(), "canario") {
		t.Fatalf("debió fallar por canario: %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el canario no arranca")
	}
	if _, serr := os.Stat(filepath.Join(base, "live.b")); !os.IsNotExist(serr) {
		t.Fatal("slot inactivo debe limpiarse")
	}
}

// Si el corte falla, el canario se mata y no se publica nada.
func TestDeployCorteFallido(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	os.MkdirAll(live, 0o755)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{failCutover: true}
	defer ctl.close()
	_, err := RunContext(context.Background(), testApp(live, repo, nil), "", ctl, io.Discard, log)
	if err == nil || !strings.Contains(err.Error(), "corte") {
		t.Fatalf("debió fallar por corte: %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("corte fallido no debe registrarse")
	}
}
