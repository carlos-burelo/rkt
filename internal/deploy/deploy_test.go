package deploy

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/supervisor"
)

func gitOK(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
}

// mkRepo crea repo git local con un commit. Devuelve path y rev.
func mkRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-b", "main", ".")
	for name, body := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
		run("add", ".")
	}
	run("commit", "-m", "v1", "--no-gpg-sign")
	return dir, run("rev-parse", "HEAD")
}

// fakeCtl ejecuta swap de verdad contra disco y simula salud.
type fakeCtl struct {
	swaps   int
	healthy bool
}

func (f *fakeCtl) SwapAndRestart(name string, swap func() error) error {
	f.swaps++
	return swap()
}

func (f *fakeCtl) Snapshot() []supervisor.Status {
	return []supervisor.Status{{Name: "web", Running: f.healthy, Failed: !f.healthy}}
}

func testApp(dir, repo string, build []string) config.App {
	return config.App{
		Name: "web", Command: "x", Dir: dir,
		Deploy: &config.Deploy{Repo: repo, Ref: "main", TokenEnv: "RKT_DEPLOY_TEST_TOKEN", Build: build},
	}
}

func TestDeployHappy(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	live := filepath.Join(t.TempDir(), "live")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "old.txt"), []byte("viejo\n"), 0o644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rev, err := Run(testApp(live, repo, nil), "", &fakeCtl{healthy: true}, io.Discard, log)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if rev == "" || rev == "?" {
		t.Fatalf("rev vacío: %q", rev)
	}
	if b, _ := os.ReadFile(filepath.Join(live, "app.txt")); strings.TrimSpace(string(b)) != "v2" {
		t.Fatalf("live sin lo nuevo: %q", b)
	}
	if _, err := os.Stat(filepath.Join(live, "old.txt")); !os.IsNotExist(err) {
		t.Fatal("live reemplazado: old.txt no debe seguir")
	}
	if b, _ := os.ReadFile(filepath.Join(live+".prev", "old.txt")); strings.TrimSpace(string(b)) != "viejo" {
		t.Fatal("backup .prev sin lo viejo")
	}
	if _, err := os.Stat(live + ".staging"); !os.IsNotExist(err) {
		t.Fatal("staging debe limpiarse")
	}
}

func TestDeployBuildFailure(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	live := filepath.Join(t.TempDir(), "live")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "old.txt"), []byte("viejo\n"), 0o644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{healthy: true}
	if _, err := Run(testApp(live, repo, []string{"exit 1"}), "", ctl, io.Discard, log); err == nil {
		t.Fatal("build roto debe fallar")
	}
	if b, _ := os.ReadFile(filepath.Join(live, "old.txt")); strings.TrimSpace(string(b)) != "viejo" {
		t.Fatal("live intacto tras build roto")
	}
	if ctl.swaps != 0 {
		t.Fatal("sin swap si el build falla")
	}
	if _, err := os.Stat(live + ".staging"); !os.IsNotExist(err) {
		t.Fatal("staging debe limpiarse")
	}
}

func TestDeployRollbackOnUnhealthy(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	live := filepath.Join(t.TempDir(), "live")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "old.txt"), []byte("viejo\n"), 0o644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{healthy: false}
	_, err := Run(testApp(live, repo, nil), "", ctl, io.Discard, log)
	if err == nil || !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("debió fallar con rollback: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(live, "old.txt")); strings.TrimSpace(string(b)) != "viejo" {
		t.Fatal("rollback: live con lo viejo")
	}
	if ctl.swaps != 2 {
		t.Fatalf("swap x2 (deploy+rollback), got %d", ctl.swaps)
	}
}

func TestDeploySHARef(t *testing.T) {
	gitOK(t)
	repo, sha := mkRepo(t, map[string]string{"app.txt": "v1\n"})
	live := filepath.Join(t.TempDir(), "live")
	os.MkdirAll(live, 0o755)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := testApp(live, repo, nil)
	app.Deploy.Ref = sha
	if _, err := Run(app, "", &fakeCtl{healthy: true}, io.Discard, log); err != nil {
		t.Fatalf("deploy por SHA: %v", err)
	}
	out, _ := exec.Command("git", "-C", live, "rev-parse", "HEAD").Output()
	if got := strings.TrimSpace(string(out)); got != sha {
		t.Fatalf("clonado en %s, quiere %s", got, sha)
	}
}

func TestDeployNeedsToken(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := testApp(t.TempDir(), "https://github.com/o/r.git", nil)
	if _, err := Run(app, "", &fakeCtl{healthy: true}, io.Discard, log); err == nil {
		t.Fatal("https sin token debe fallar sin tocar red")
	}
}
