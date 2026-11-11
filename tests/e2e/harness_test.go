//go:build e2e

package e2e_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"rkt-proxy/internal/api"
	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dashboard"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/hub"
	"rkt-proxy/internal/store"
)

const adminPassword = "password-larga-1"

type browserApp struct {
	url     string
	db      *store.DB
	live    *config.Live
	browser playwright.Browser
}

func newBrowserApp(t *testing.T) *browserApp {
	return newBrowserAppWith(t, nil)
}

func newBrowserAppWith(t *testing.T, mut func(*api.Deps)) *browserApp {
	t.Helper()
	dist := filepath.Join("..", "..", "web", "dist")
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Fatalf("falta dashboard: pnpm --dir web build: %v", err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := &config.Config{}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := cfgfile.SaveFleet(db, cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfgfile.SaveInfra(db, cfg); err != nil {
		t.Fatal(err)
	}
	live := config.NewLive(cfg)
	bus := hub.New()
	deps := api.Deps{DB: db, Live: live, Hub: bus, Version: "e2e", LogsDir: t.TempDir(), Apps: func() []health.AppStatus { return nil }}
	if mut != nil {
		mut(&deps)
	}
	deps.Apply = func(_ []config.App, _ map[string]string) {
		current := live.Get()
		metas := make([]map[string]any, 0, len(current.Apps))
		for _, app := range current.Apps {
			metas = append(metas, appview.App(current, app))
		}
		bus.Publish(hub.TopicMetas, hub.TopicMetas, metas)
		bus.Publish(hub.TopicStatus, hub.TopicStatus, map[string]any{
			"version": "e2e", "routes": len(current.Routes), "apps": []health.AppStatus{},
		})
	}
	apiHandler := api.NewHandler(deps)
	webHandler := dashboard.NewHandler(os.DirFS(dist))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		webHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("iniciar Playwright (instala driver y Chromium: go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6401.0 install chromium): %v", err)
	}
	t.Cleanup(func() { _ = pw.Stop() })
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatalf("abrir Chromium: %v", err)
	}
	t.Cleanup(func() { _ = browser.Close() })
	return &browserApp{url: server.URL, db: db, live: live, browser: browser}
}

func (a *browserApp) setup(t *testing.T) playwright.Page {
	t.Helper()
	page, err := a.browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(a.url + "/overview"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/setup"); err != nil {
		t.Fatalf("sin usuarios debe abrir setup: %v", err)
	}
	for i, value := range []string{"jefe", adminPassword, adminPassword} {
		if err := page.Locator("form input").Nth(i).Fill(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := page.Locator("form button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	// Stepper: admin creado → red (omitir) → herramientas (omitir) →
	// GitHub (opt-out consciente) → fin → overview.
	omitir := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Omitir y continuar"})
	if err := omitir.First().Click(); err != nil {
		t.Fatalf("stepper red: %v", err)
	}
	if err := omitir.First().Click(); err != nil {
		t.Fatalf("stepper herramientas: %v", err)
	}
	if err := page.GetByRole("checkbox").First().Check(); err != nil {
		t.Fatalf("opt-out github: %v", err)
	}
	if err := omitir.First().Click(); err != nil {
		t.Fatalf("stepper github: %v", err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Entrar en rkt"}).Click(); err != nil {
		t.Fatalf("stepper fin: %v", err)
	}
	if err := page.WaitForURL("**/overview"); err != nil {
		t.Fatalf("setup no inició sesión: %v", err)
	}
	return page
}

func (a *browserApp) login(t *testing.T, user, password string) playwright.Page {
	t.Helper()
	page, err := a.browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(a.url + "/overview"); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/login"); err != nil {
		t.Fatalf("sesión nueva debe pedir login: %v", err)
	}
	if err := page.Locator("form input").Nth(0).Fill(user); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("form input").Nth(1).Fill(password); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("form button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/overview"); err != nil {
		t.Fatalf("login no abrió el dashboard: %v", err)
	}
	return page
}
