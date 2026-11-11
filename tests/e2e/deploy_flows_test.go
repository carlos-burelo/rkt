//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
	"rkt-proxy/internal/api"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

// Caminos de deploy de punta a punta contra el servidor real (misma API que
// usa el dashboard y el CLI): sync/async, build local, rollback, retry,
// cancel y el 409 de apps externas. Los stubs simulan el supervisor; lo que
// se corrobora es el enrutado, los roles, los códigos y el historial.

// deployHooks stubs controlables por test (cierran sobre vars del caller).
type deployHooks struct {
	deployErr error
	buildID   int64
	buildErr  error
	rollErr   error
	retryRev  string
	retryErr  error
	cancelOK  bool
}

func (h *deployHooks) apply(d *api.Deps) {
	d.Deploy = func(name, actor string) (string, error) {
		if h.deployErr != nil {
			return "", h.deployErr
		}
		if actor == "" {
			return "", fmt.Errorf("sin actor")
		}
		return "deadbeef", nil
	}
	d.DeployAsync = func(name, actor string) (int64, error) { return 42, nil }
	d.Build = func(name, actor string) (int64, error) {
		if h.buildErr != nil {
			return 0, h.buildErr
		}
		return h.buildID, nil
	}
	d.Rollback = func(name, actor string) error { return h.rollErr }
	d.Retry = func(id int64, actor string) (string, error) {
		if h.retryErr != nil {
			return "", h.retryErr
		}
		return h.retryRev, nil
	}
	d.Cancel = func(id int64) bool { return h.cancelOK }
	d.Restart = func(name string) bool { return true }
}

// seedFleet añade gitapp (con repo), localapp (sin repo) y extapp (externa).
func seedFleet(t *testing.T, app *browserApp) {
	t.Helper()
	mkDir := func() string {
		d := t.TempDir()
		return d
	}
	cp, err := cfgfile.MutateFleet(app.db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps,
			config.App{
				Name: "gitapp", Command: "node", Args: []string{"s.js"},
				Dir: mkDir(), Env: map[string]string{"PORT": "3101"},
				RestartDelay: "5s", MinUptime: "10s",
				Deploy: &config.Deploy{Repo: "https://github.com/o/r.git", Ref: "main"},
			},
			config.App{
				Name: "localapp", Command: "node", Args: []string{"s.js"},
				Dir: mkDir(), Env: map[string]string{"PORT": "3102"},
				RestartDelay: "5s", MinUptime: "10s",
			},
			config.App{
				Name: "extapp", Runtime: config.RuntimeExternal,
				Dir: mkDir(), Env: map[string]string{"PORT": "3103"},
			},
		)
		cp.Routes = append(cp.Routes,
			config.Route{Hosts: []string{"gitapp.test"}, Target: "http://127.0.0.1:3101"},
			config.Route{Hosts: []string{"localapp.test"}, Target: "http://127.0.0.1:3102"},
			config.Route{Hosts: []string{"extapp.test"}, Target: "http://127.0.0.1:3103"},
		)
		return nil
	})
	if err != nil {
		t.Fatalf("seed flota: %v", err)
	}
	app.live.Swap(cp)
}

type httpClient struct {
	t   *testing.T
	url string
	tok string
}

func (c *httpClient) do(method, path string, body any) (int, string) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		switch b := body.(type) {
		case string:
			r = strings.NewReader(b)
		default:
			j, _ := json.Marshal(body)
			r = bytes.NewReader(j)
		}
	}
	req, err := http.NewRequest(method, c.url+path, r)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func opClient(t *testing.T, app *browserApp) *httpClient {
	t.Helper()
	if err := app.db.CreateUser("op", "password-larga-1", store.RoleOperator); err != nil {
		t.Fatal(err)
	}
	tok, err := app.db.NewSession("op", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &httpClient{t: t, url: app.url, tok: tok}
}

func TestDeploySyncOKYAsync(t *testing.T) {
	h := &deployHooks{buildID: 7}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	// sync con repo -> stub devuelve rev
	if code, res := c.do(http.MethodPost, "/api/v1/apps/gitapp/deploy", nil); code != http.StatusOK || !strings.Contains(res, "deadbeef") {
		t.Fatalf("deploy sync: %d %s", code, res)
	}
	// async -> 202 con id
	if code, res := c.do(http.MethodPost, "/api/v1/apps/gitapp/deploy?async=1", nil); code != http.StatusAccepted || !strings.Contains(res, "42") {
		t.Fatalf("deploy async: %d %s", code, res)
	}
	// sin repo -> rebuild vía Build (202)
	if code, res := c.do(http.MethodPost, "/api/v1/apps/localapp/deploy", nil); code != http.StatusAccepted || !strings.Contains(res, `"deployment":7`) {
		t.Fatalf("deploy local rebuild: %d %s", code, res)
	}
}

func TestDeploySyncFalla500(t *testing.T) {
	h := &deployHooks{deployErr: fmt.Errorf("clone: red caída")}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	if code, res := c.do(http.MethodPost, "/api/v1/apps/gitapp/deploy", nil); code != http.StatusInternalServerError || !strings.Contains(res, "red caída") {
		t.Fatalf("deploy roto: %d %s", code, res)
	}
}

func TestBuildLocalCaminos(t *testing.T) {
	h := &deployHooks{buildID: 9}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	// build ok
	if code, res := c.do(http.MethodPost, "/api/v1/apps/localapp/build", nil); code != http.StatusOK || !strings.Contains(res, `"deployment":9`) {
		t.Fatalf("build: %d %s", code, res)
	}
	// build con error del backend
	h.buildErr = fmt.Errorf("pnpm: comando fallido")
	if code, _ := c.do(http.MethodPost, "/api/v1/apps/localapp/build", nil); code != http.StatusInternalServerError {
		t.Fatalf("build roto: quiere 500, got %d", code)
	}
	h.buildErr = nil
	// externa -> 409
	if code, _ := c.do(http.MethodPost, "/api/v1/apps/extapp/build", nil); code != http.StatusConflict {
		t.Fatalf("build externa: quiere 409, got %d", code)
	}
	// fantasma -> 404
	if code, _ := c.do(http.MethodPost, "/api/v1/apps/nadie/build", nil); code != http.StatusNotFound {
		t.Fatalf("build fantasma: quiere 404, got %d", code)
	}
}

func TestRollbackCaminos(t *testing.T) {
	h := &deployHooks{}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	// rollback ok
	if code, res := c.do(http.MethodPost, "/api/v1/apps/gitapp/rollback", nil); code != http.StatusOK || !strings.Contains(res, "true") {
		t.Fatalf("rollback: %d %s", code, res)
	}
	// rollback con error
	h.rollErr = fmt.Errorf("sin backup")
	if code, _ := c.do(http.MethodPost, "/api/v1/apps/gitapp/rollback", nil); code != http.StatusInternalServerError {
		t.Fatalf("rollback roto: quiere 500, got %d", code)
	}
	h.rollErr = nil
	// externa -> 409
	if code, _ := c.do(http.MethodPost, "/api/v1/apps/extapp/rollback", nil); code != http.StatusConflict {
		t.Fatalf("rollback externa: quiere 409, got %d", code)
	}
}

func TestRetryCancelCaminos(t *testing.T) {
	h := &deployHooks{retryRev: "rev-retry", cancelOK: true}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	id, err := app.db.InsertDeployment("gitapp", "", "", "op")
	if err != nil {
		t.Fatal(err)
	}
	retryPath := fmt.Sprintf("/api/v1/deployments/%d/retry", id)

	// retry ok
	if code, res := c.do(http.MethodPost, retryPath, nil); code != http.StatusOK || !strings.Contains(res, "rev-retry") {
		t.Fatalf("retry: %d %s", code, res)
	}
	// retry con error del backend
	h.retryErr = fmt.Errorf("rev sin repetible")
	if code, _ := c.do(http.MethodPost, retryPath, nil); code != http.StatusBadRequest {
		t.Fatalf("retry roto: quiere 400, got %d", code)
	}
	h.retryErr = nil
	// retry id inválido
	if code, _ := c.do(http.MethodPost, "/api/v1/deployments/abc/retry", nil); code != http.StatusBadRequest {
		t.Fatalf("retry id malo: quiere 400, got %d", code)
	}

	// cancel ok (en curso) y cancel cuando no hay nada (409)
	cancelPath := fmt.Sprintf("/api/v1/deployments/%d/cancel", id)
	if code, _ := c.do(http.MethodPost, cancelPath, nil); code != http.StatusOK {
		t.Fatalf("cancel: quiere 200, got %d", code)
	}
	h.cancelOK = false
	if code, _ := c.do(http.MethodPost, cancelPath, nil); code != http.StatusConflict {
		t.Fatalf("cancel sin curso: quiere 409, got %d", code)
	}
}

func TestDeployExterna409(t *testing.T) {
	h := &deployHooks{}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	for _, p := range []string{
		"/api/v1/apps/extapp/deploy",
		"/api/v1/apps/extapp/restart",
		"/api/v1/apps/extapp/rollback",
		"/api/v1/apps/extapp/build",
		"/api/v1/apps/extapp/start",
		"/api/v1/apps/extapp/stop",
	} {
		if code, _ := c.do(http.MethodPost, p, nil); code != http.StatusConflict {
			t.Fatalf("POST %s externa: quiere 409, got %d", p, code)
		}
	}
}

func TestRestartStartStop(t *testing.T) {
	h := &deployHooks{}
	app := newBrowserAppWith(t, h.apply)
	_ = app.setup(t)
	seedFleet(t, app)
	c := opClient(t, app)

	if code, _ := c.do(http.MethodPost, "/api/v1/apps/gitapp/restart", nil); code != http.StatusOK {
		t.Fatalf("restart: quiere 200, got %d", code)
	}
	if code, res := c.do(http.MethodPost, "/api/v1/apps/gitapp/stop", nil); code != http.StatusOK || !strings.Contains(res, "true") {
		t.Fatalf("stop: %d %s", code, res)
	}
	if code, res := c.do(http.MethodPost, "/api/v1/apps/gitapp/start", nil); code != http.StatusOK {
		t.Fatalf("start: %d %s", code, res)
	}
}

// El tablero /deployments muestra el historial real: inserta una fila y
// exige el rev en el DOM (misma API que consume useFleet).
func TestBrowserDeploymentsMuestranHistorial(t *testing.T) {
	h := &deployHooks{}
	app := newBrowserAppWith(t, h.apply)
	page := app.setup(t)
	seedFleet(t, app)

	if _, err := app.db.InsertDeployment("gitapp", "cafef00d", "", "op"); err != nil {
		t.Fatal(err)
	}

	if _, err := page.Goto(app.url + "/deployments"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/deployments"); err != nil {
		t.Fatalf("no abrió deployments: %v", err)
	}
	// El rev vive en el DOM (celda desktop + resumen móvil); basta attached.
	rev := page.Locator("text=cafef00d")
	if err := rev.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateAttached}); err != nil {
		t.Fatalf("el tablero no muestra el deploy: %v", err)
	}
	if n, err := rev.Count(); err != nil || n == 0 {
		t.Fatalf("sin nodos con el rev: %d %v", n, err)
	}
}
