//go:build e2e

package e2e_test

// Deploy real de punta a punta: repo git de verdad (fixture demo-py),
// pipeline real (clone+build+canario+corte) con supervisor real, historial
// como startDeploy, y el navegador exige el rev en el DOM (overview de la
// app y tablero de deployments) más el contenido servido por el proxy.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"rkt-proxy/internal/api"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/proxy"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/supervisor"
)

func e2ePython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		if path, err := exec.LookPath(name); err == nil {
			if exec.Command(path, "--version").Run() == nil {
				return path
			}
		}
	}
	t.Skip("sin Python ejecutable")
	return ""
}

func e2eGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
}

// e2eFixtureRepo copia tests/apps/<name> y lo vuelve repo git.
func e2eFixtureRepo(t *testing.T, name string) string {
	t.Helper()
	e2eGit(t)
	src := filepath.Join("..", "apps", name)
	if env := os.Getenv("RKT_TEST_APPS"); env != "" {
		src = filepath.Join(env, name)
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		t.Fatalf("fixture %s ausente", name)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	copyTreeE2E(t, src, repo)
	home := t.TempDir()
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+home,
	)
	for _, args := range [][]string{{"init", "-b", "main", "."}, {"add", "."}, {"commit", "-m", "v1", "--no-gpg-sign"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return repo
}

func copyTreeE2E(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		s, d := filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
			copyTreeE2E(t, s, d)
			continue
		}
		b, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// e2eCtl Controller real sobre supervisor + DB + Live.
type e2eCtl struct {
	sup  *supervisor.Manager
	db   *store.DB
	live *config.Live
	log  *slog.Logger
}

func (c *e2eCtl) SwapAndRestart(name string, swap func() error) error {
	return c.sup.SwapAndRestart(name, swap)
}

func (c *e2eCtl) Snapshot() []supervisor.Status { return c.sup.Snapshot() }

func (c *e2eCtl) StartCanary(name, dir, port string, out io.Writer) (*exec.Cmd, error) {
	return c.sup.StartCanary(name, dir, port, out)
}

func (c *e2eCtl) Cutover(name string, cmd *exec.Cmd, dir, port string) error {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	cp, err := cfgfile.MutateFleet(c.db, func(cp *config.Config) error {
		return cfgfile.UpdateApp(cp, name, cfgfile.AppPatch{Dir: &dir, Port: &portInt})
	})
	if err != nil {
		return err
	}
	c.live.Swap(cp)
	return c.sup.SwapLive(name, cmd, dir, port)
}

func TestBrowserDeployReal(t *testing.T) {
	py := e2ePython(t)
	repo := e2eFixtureRepo(t, "demo-py")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	var sup *supervisor.Manager
	mut := func(d *api.Deps) {
		cp, err := cfgfile.MutateFleet(d.DB, func(cp *config.Config) error {
			cp.Apps = append(cp.Apps, config.App{
				Name: "tienda", Command: py, Args: []string{"-u", "app.py"},
				Dir: filepath.Join(t.TempDir(), "tienda.a"), Env: map[string]string{"PORT": "1"},
				RestartDelay: "5s", MinUptime: "1s",
				Deploy: &config.Deploy{Repo: repo, Ref: "main", Build: []string{"python -m py_compile app.py"}},
			})
			cp.Routes = append(cp.Routes, config.Route{Hosts: []string{"tienda.test"}, Target: "http://127.0.0.1:1"})
			return nil
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		d.Live.Swap(cp)
		sup = supervisor.New(cp.Apps, nil, filepath.Join(t.TempDir(), "logs"), log)
		t.Cleanup(sup.Stop)
		ctl := &e2eCtl{sup: sup, db: d.DB, live: d.Live, log: log}
		// Como startDeploy: historial + pipeline real.
		d.Deploy = func(name, actor string) (string, error) {
			var app config.App
			for _, a := range d.Live.Get().Apps {
				if a.Name == name {
					app = a
				}
			}
			if app.Name == "" {
				return "", fmt.Errorf("app %q inexistente", name)
			}
			id, _ := d.DB.InsertDeployment(name, "", "", actor)
			rev, err := deploy.RunContext(context.Background(), app, "", ctl, io.Discard, log)
			status := "ok"
			if err != nil {
				status = "failed"
			}
			if id > 0 {
				_ = d.DB.FinishDeployment(id, status, "", rev)
			}
			return rev, err
		}
	}
	app := newBrowserAppWith(t, mut)
	page := app.setup(t)

	// Trigger por API (mismo contrato que el botón del dashboard).
	cli := opClient(t, app)
	code, body := cli.do(http.MethodPost, "/api/v1/apps/tienda/deploy", nil)
	if code != http.StatusOK {
		t.Fatalf("deploy: %d %s", code, body)
	}
	var got struct {
		Rev string `json:"rev"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.Rev == "" || got.Rev == "?" {
		t.Fatalf("rev vacío: %s %v", body, err)
	}
	rev := got.Rev

	// El proxy sirve la versión desplegada por su host.
	front := httptest.NewServer(proxy.NewHandler(app.live, log))
	defer front.Close()
	req, _ := http.NewRequest("GET", front.URL, nil)
	req.Host = "tienda.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "v1" {
		t.Fatalf("proxy sirvió %q, quiere v1", b)
	}

	// El tablero muestra el rev real en el DOM.
	if _, err := page.Goto(app.url + "/app/tienda/overview"); err != nil {
		t.Fatal(err)
	}
	marker := rev
	if len(marker) > 8 {
		marker = marker[:8]
	}
	loc := page.Locator("text=" + marker)
	if err := loc.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatalf("overview no muestra el rev %q", marker)
	}
	if _, err := page.Goto(app.url + "/deployments"); err != nil {
		t.Fatal(err)
	}
	loc2 := page.Locator("text=" + marker)
	if err := loc2.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatalf("deployments no muestra el rev %q", marker)
	}
}
