package discover

// Detección por manifiestos sin GitHub App: clone sparse de un repo,
// DetectDir sobre lo traído, temp limpiado. Sin red (repos locales).

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"rkt-proxy/internal/store"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
}

func mkRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main", ".")
	for name, body := range files {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "v1", "--no-gpg-sign")
	return dir
}

func TestDetectPlanShallowNode(t *testing.T) {
	gitOK(t)
	repo := mkRepo(t, map[string]string{
		"package.json": `{"scripts":{"build":"vite build","start":"node server.js"}}`,
		"server.js":    "x",
		// señuelo pesado: el sparse no debe traerlo (y ni haría falta).
		"dist/bundle.js": "x",
	})
	p, err := detectPlanShallow(repo)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind == "" {
		t.Fatal("sin plan en repo node")
	}
	if len(p.Start) == 0 {
		t.Fatalf("plan sin arranque: %+v", p)
	}
}

func TestDetectPlanShallowStatic(t *testing.T) {
	gitOK(t)
	repo := mkRepo(t, map[string]string{"public/index.html": "<p>hola</p>"})
	p, err := detectPlanShallow(repo)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "static" {
		t.Fatalf("quiere static, got %+v", p)
	}
}

func TestDetectPlanShallowSinManifiesto(t *testing.T) {
	gitOK(t)
	repo := mkRepo(t, map[string]string{"README.md": "nada que detectar"})
	if _, err := detectPlanShallow(repo); err == nil {
		t.Fatal("repo sin manifiestos debe fallar")
	}
}

func TestDetectPlanShallowSinGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err == nil {
		t.Skip("hay git; este caso es para PATH sin git")
	}
	if _, err := detectPlanShallow("/no/existe"); err == nil {
		t.Fatal("sin git debe fallar")
	}
}

func TestDetectPlanShallowRepoInexistente(t *testing.T) {
	gitOK(t)
	if _, err := detectPlanShallow(filepath.Join(t.TempDir(), "noexiste")); err == nil {
		t.Fatal("repo inexistente debe fallar")
	}
}

func TestSuggestSinGitHubDegradaSinError(t *testing.T) {
	gitOK(t)
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// Host .invalid (RFC): DNS falla al instante, sin red de verdad.
	// Sin GitHub App y sin plan detectable: ni error ni crash, plan vacío.
	s, err := Suggest(db, nil, "https://novalido.invalid/o/r.git")
	if err != nil {
		t.Fatalf("debe degradar sin error: %v", err)
	}
	if s.Preset != "" || s.Plan.Kind != "" {
		t.Fatalf("sin plan esperado, got %+v", s.Plan)
	}
	if s.Name != "r" {
		t.Fatalf("slug = %q", s.Name)
	}
}
