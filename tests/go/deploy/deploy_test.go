package deploy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/supervisor"
)

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

// fakeCtl simula el controller para el blue-green: arranca un mini servidor
// HTTP en el puerto del canario y registra el corte.
type fakeCtl struct {
	swaps       int
	cutoverDir  string
	cutoverPort string
	noHTTP      bool // si true, StartCanary no arranca el server (canario mudo)
	failCanary  bool // si true, StartCanary falla
	failCutover bool // si true, Cutover falla
	srvs        []*http.Server
	lns         []net.Listener
}

func (f *fakeCtl) SwapAndRestart(name string, swap func() error) error {
	f.swaps++
	return swap()
}

func (f *fakeCtl) Snapshot() []supervisor.Status {
	return []supervisor.Status{{Name: "web", Running: true}}
}

// StartCanary sirve 200 en el puerto (simula el canario sano) y devuelve un
// proceso que duerme (para que Kill/Cutover tengan Process no-nil).
func (f *fakeCtl) StartCanary(name, dir, port string, out io.Writer) (*exec.Cmd, error) {
	if f.failCanary {
		return nil, fmt.Errorf("canario no arrancó: simulado")
	}
	if !f.noHTTP {
		ln, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err != nil {
			return nil, err
		}
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})}
		go srv.Serve(ln)
		f.srvs = append(f.srvs, srv)
		f.lns = append(f.lns, ln)
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/c", "ping -n 1000 127.0.0.1 >nul")
	} else {
		cmd = exec.Command("sleep", "1000")
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (f *fakeCtl) Cutover(name string, cmd *exec.Cmd, dir, port string) error {
	if f.failCutover {
		return fmt.Errorf("corte fallido: simulado")
	}
	f.cutoverDir = dir
	f.cutoverPort = port
	return nil
}

func (f *fakeCtl) close() {
	for _, s := range f.srvs {
		_ = s.Close()
	}
	for _, l := range f.lns {
		_ = l.Close()
	}
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
	live := filepath.Join(t.TempDir(), "live.a")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "old.txt"), []byte("viejo\n"), 0o644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{}
	defer ctl.close()
	rev, err := Run(testApp(live, repo, nil), "", ctl, io.Discard, log)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if rev == "" || rev == "?" {
		t.Fatalf("rev vacío: %q", rev)
	}
	inactive := filepath.Join(filepath.Dir(live), "live.b")
	if b, _ := os.ReadFile(filepath.Join(inactive, "app.txt")); strings.TrimSpace(string(b)) != "v2" {
		t.Fatalf("slot inactivo sin lo nuevo: %q", b)
	}
	if ctl.cutoverDir != inactive {
		t.Fatalf("corte a %q, quiere %q", ctl.cutoverDir, inactive)
	}
	if ctl.cutoverPort == "" {
		t.Fatal("corte sin puerto")
	}
	// el slot activo (viejo) queda intacto hasta el corte
	if b, _ := os.ReadFile(filepath.Join(live, "old.txt")); strings.TrimSpace(string(b)) != "viejo" {
		t.Fatal("slot activo intacto durante el corte")
	}
}

func TestDeployBuildFailure(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	live := filepath.Join(t.TempDir(), "live.a")
	os.MkdirAll(live, 0o755)
	os.WriteFile(filepath.Join(live, "old.txt"), []byte("viejo\n"), 0o644)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{}
	defer ctl.close()
	if _, err := Run(testApp(live, repo, []string{"exit 1"}), "", ctl, io.Discard, log); err == nil {
		t.Fatal("build roto debe fallar")
	}
	if b, _ := os.ReadFile(filepath.Join(live, "old.txt")); strings.TrimSpace(string(b)) != "viejo" {
		t.Fatal("slot activo intacto tras build roto")
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el build falla")
	}
}

func TestDeployCanarioMudo(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"app.txt": "v2\n"})
	live := filepath.Join(t.TempDir(), "live.a")
	os.MkdirAll(live, 0o755)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctl := &fakeCtl{noHTTP: true}
	defer ctl.close()
	_, err := Run(testApp(live, repo, nil), "", ctl, io.Discard, log)
	if err == nil || !strings.Contains(err.Error(), "canario") {
		t.Fatalf("debió fallar por canario mudo: %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el canario no responde")
	}
}

func TestDeploySHARef(t *testing.T) {
	gitOK(t)
	repo, sha := mkRepo(t, map[string]string{"app.txt": "v1\n"})
	live := filepath.Join(t.TempDir(), "live.a")
	os.MkdirAll(live, 0o755)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := testApp(live, repo, nil)
	app.Deploy.Ref = sha
	ctl := &fakeCtl{}
	defer ctl.close()
	if _, err := Run(app, "", ctl, io.Discard, log); err != nil {
		t.Fatalf("deploy por SHA: %v", err)
	}
	inactive := filepath.Join(filepath.Dir(live), "live.b")
	out, _ := exec.Command("git", "-C", inactive, "rev-parse", "HEAD").Output()
	if got := strings.TrimSpace(string(out)); got != sha {
		t.Fatalf("clonado en %s, quiere %s", got, sha)
	}
}

func TestDeployNeedsToken(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := testApp(t.TempDir(), "https://github.com/o/r.git", nil)
	if _, err := Run(app, "", &fakeCtl{}, io.Discard, log); err == nil {
		t.Fatal("https sin token debe fallar sin tocar red")
	}
}

func TestOtherSlot(t *testing.T) {
	cases := map[string]string{
		"/apps/x.a": "/apps/x.b",
		"/apps/x.b": "/apps/x.a",
		"/apps/x":   "/apps/x.b", // app antigua: primer blue-green va a .b
	}
	for in, want := range cases {
		if got := OtherSlot(in); got != want {
			t.Fatalf("OtherSlot(%q) = %q, quiere %q", in, got, want)
		}
	}
}
