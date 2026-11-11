//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
	"rkt-proxy/internal/store"
)

// Smoke de navegación: cada ruta principal carga sin el fallback de error
// ("error de interfaz") y sin rebotar al login una vez autenticado.
func TestBrowserNavSmoke(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)

	routes := []string{
		"/overview", "/deployments", "/ci", "/logs", "/cron",
		"/analytics", "/observability", "/server", "/alerts",
		"/environment", "/routes", "/settings", "/users",
	}
	for _, r := range routes {
		if _, err := page.Goto(app.url + r); err != nil {
			t.Fatalf("goto %s: %v", r, err)
		}
		if err := page.WaitForURL("**" + r); err != nil {
			t.Fatalf("%s no se quedó en ruta (¿rebote a login?): %v", r, err)
		}
		body, err := page.Content()
		if err != nil {
			t.Fatalf("contenido %s: %v", r, err)
		}
		if strings.Contains(body, "error de interfaz") {
			t.Fatalf("%s pintó el fallback de error", r)
		}
	}
}

// Gating por rol en navegador real: viewer no entra a /users (rebota a
// /overview) y el admin sí lo abre y ve al usuario creado.
func TestBrowserRoleGating(t *testing.T) {
	app := newBrowserApp(t)
	adminPage := app.setup(t)

	if err := app.db.CreateUser("miron", "password-larga-1", store.RoleViewer); err != nil {
		t.Fatal(err)
	}

	viewerPage := app.login(t, "miron", "password-larga-1")
	if _, err := viewerPage.Goto(app.url + "/users"); err != nil {
		t.Fatal(err)
	}
	// UsersRoute redirige a /overview para no-admin
	if err := viewerPage.WaitForURL("**/overview"); err != nil {
		t.Fatalf("viewer debió rebotar de /users a /overview: %v", err)
	}

	if _, err := adminPage.Goto(app.url + "/users"); err != nil {
		t.Fatal(err)
	}
	if err := adminPage.WaitForURL("**/users"); err != nil {
		t.Fatalf("admin no abrió /users: %v", err)
	}
	// la lista carga async (useEffect): espera al nombre en vez de leer de golpe
	miron := adminPage.Locator("text=miron")
	if err := miron.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}); err != nil {
		t.Fatalf("/users no muestra al usuario creado: %v", err)
	}
}

// Flujo HTTP completo contra el servidor real (httptest + SQLite real):
// app externa -> env -> routes -> alerts -> cron -> server -> proxy.
// Corrobora el sistema de punta a punta sin navegador.
func TestSystemFlowHTTP(t *testing.T) {
	app := newBrowserApp(t)
	_ = app.setup(t)

	if err := app.db.CreateUser("op", "password-larga-1", store.RoleOperator); err != nil {
		t.Fatal(err)
	}
	tok, err := app.db.NewSession("op", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path string, body any) (int, string) {
		t.Helper()
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, app.url+path, r)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// 1. alta de app externa (sin repo: dir local + puerto sugerido)
	extDir := t.TempDir()
	code, res := do(http.MethodPost, "/api/v1/apps", map[string]any{
		"runtime": "external", "name": "ext1", "dir": extDir,
		"domains": []string{"ext1.test"},
	})
	if code != http.StatusCreated || !strings.Contains(res, "ext1") {
		t.Fatalf("alta app externa: %d %s", code, res)
	}

	// 2. detalle + env set + lectura de valor
	if code, res := do(http.MethodGet, "/api/v1/apps/ext1", nil); code != http.StatusOK {
		t.Fatalf("detalle app: %d %s", code, res)
	}
	if code, res := do(http.MethodPut, "/api/v1/apps/ext1/env", map[string]any{
		"set": map[string]string{"K": "v"},
	}); code != http.StatusOK {
		t.Fatalf("env set: %d %s", code, res)
	}
	if code, res := do(http.MethodGet, "/api/v1/apps/ext1/env/K", nil); code != http.StatusOK || !strings.Contains(res, `"value":"v"`) {
		t.Fatalf("env valor: %d %s", code, res)
	}

	// 3. routes: la app creó su ruta; listado la trae
	if code, res := do(http.MethodGet, "/api/v1/routes", nil); code != http.StatusOK || !strings.Contains(res, "ext1.test") {
		t.Fatalf("routes: %d %s", code, res)
	}

	// 4. alertas: canal + regla + listado
	if code, res := do(http.MethodPost, "/api/v1/alert-channels", map[string]any{
		"name": "c", "url": "https://example.com/hook",
	}); code != http.StatusCreated {
		t.Fatalf("canal: %d %s", code, res)
	}
	if code, res := do(http.MethodPost, "/api/v1/alerts", map[string]any{
		"app": "ext1", "kind": "failed", "threshold": 1, "window_min": 60,
	}); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("regla: %d %s", code, res)
	}
	if code, res := do(http.MethodGet, "/api/v1/alerts", nil); code != http.StatusOK || !strings.Contains(res, "ext1") {
		t.Fatalf("alertas lista: %d %s", code, res)
	}

	// 5. cron: alta de job + listado + borrado
	cronPut := map[string]any{"schedule": "* * * * *", "command": "echo e2e", "timeout": "1m"}
	req, _ := http.NewRequest(http.MethodPut, app.url+"/api/v1/cron/ext1/e2e", nil)
	b, _ := json.Marshal(cronPut)
	req.Body = io.NopCloser(bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cron put: %d", resp.StatusCode)
	}
	if code, res := do(http.MethodGet, "/api/v1/cron", nil); code != http.StatusOK || !strings.Contains(res, `"job":"e2e"`) {
		t.Fatalf("cron lista: %d %s", code, res)
	}
	if code, _ := do(http.MethodDelete, "/api/v1/cron/ext1/e2e", nil); code != http.StatusOK {
		t.Fatalf("cron delete: %d", code)
	}

	// 6. servidor + proxy + dominio + settings
	for _, p := range []string{"/api/v1/server", "/api/v1/proxy/capabilities", "/api/v1/settings", "/api/v1/status", "/api/v1/events?n=5", "/api/v1/deployments?n=5"} {
		if code, res := do(http.MethodGet, p, nil); code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", p, code, res)
		}
	}
	if code, res := do(http.MethodGet, "/api/v1/domains/ext1.test", nil); code != http.StatusOK {
		t.Fatalf("dominio detalle: %d %s", code, res)
	}

	// 7. usuario viewer creado vía API no puede mutar env
	if err := app.db.CreateUser("v2", "password-larga-1", store.RoleViewer); err != nil {
		t.Fatal(err)
	}
	vtok, err := app.db.NewSession("v2", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req2, _ := http.NewRequest(http.MethodPut, app.url+"/api/v1/apps/ext1/env", strings.NewReader(`{"set":{"X":"1"}}`))
	req2.Header.Set("Authorization", "Bearer "+vtok)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer mutó env: %d", resp2.StatusCode)
	}
}
