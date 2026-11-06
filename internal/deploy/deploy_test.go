package deploy

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/supervisor"
)

func init() { stableFor = 10 * time.Millisecond }

func TestPreservePersist(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "live")
	staging := filepath.Join(root, "staging")
	os.MkdirAll(filepath.Join(live, "data"), 0o755)
	os.MkdirAll(filepath.Join(staging, "data"), 0o755)
	os.WriteFile(filepath.Join(live, "data", "db.sqlite"), []byte("persistente"), 0o644)
	os.WriteFile(filepath.Join(staging, "data", "db.sqlite"), []byte("nuevo"), 0o644)
	app := config.App{Name: "a", Persist: []string{"data"}}
	if err := preservePersist(app, live, staging); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(staging, "data", "db.sqlite"))
	if err != nil || string(b) != "persistente" {
		t.Fatalf("no preservó data: %q %v", b, err)
	}
	bad := config.App{Name: "a", Persist: []string{"../fuera"}}
	if err := preservePersist(bad, live, staging); err == nil {
		t.Fatal("persist traversal debe fallar")
	}
}

// TestGitTokenNoVaEnArgv: el token viaja en el entorno del hijo, no en la
// línea de comandos visible desde la lista de procesos.
func TestGitTokenNoVaEnArgv(t *testing.T) {
	cmd := gitCmd(context.Background(), "secreto-super-largo", "clone", "https://github.com/o/r.git", "dst")
	for _, a := range cmd.Args {
		if strings.Contains(a, "secreto-super-largo") {
			t.Fatalf("token en argv: %v", cmd.Args)
		}
	}
	found := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic ") {
			found = true
		}
	}
	if !found {
		t.Fatal("falta GIT_CONFIG_VALUE_0 con la credencial")
	}
}

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

func TestDiffFilesEntreRevisiones(t *testing.T) {
	gitOK(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main", ".")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	run("add", ".")
	run("commit", "-m", "v1", "--no-gpg-sign")
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	run("add", ".")
	run("commit", "-m", "v2", "--no-gpg-sign")

	changes, err := DiffFiles(dir, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Status != "A" || changes[0].Path != "b.txt" {
		t.Fatalf("cambios = %+v", changes)
	}
	if _, err := DiffFiles(dir, "--help", "HEAD"); err == nil {
		t.Fatal("revisión con flag debe rechazarse")
	}
}

func TestIsSHAAceptaRevCorto(t *testing.T) {
	for _, s := range []string{"3cb6176", "6360d12", "0123456789abcdef0123456789abcdef01234567"} {
		if !isSHA(s) {
			t.Fatalf("%q debe reconocerse como commit", s)
		}
	}
	for _, s := range []string{"", "main", "v1.2", "3cb61", "zzzzzzz"} {
		if isSHA(s) {
			t.Fatalf("%q no es commit", s)
		}
	}
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
