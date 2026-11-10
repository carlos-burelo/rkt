//go:build e2e

package e2e_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"rkt-proxy/internal/api"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dashboard"
	"rkt-proxy/internal/store"
)

// Ejercita el dashboard compilado contra la API y SQLite reales.
func TestBrowserSetupAndLogin(t *testing.T) {
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
	apiHandler := api.NewHandler(api.Deps{DB: db, Live: config.NewLive(cfg), Version: "e2e", LogsDir: t.TempDir()})
	webHandler := dashboard.NewHandler(os.DirFS(dist))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) >= len("/api/v1/") && r.URL.Path[:len("/api/v1/")] == "/api/v1/" {
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

	setupPage, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setupPage.Goto(server.URL + "/overview"); err != nil {
		t.Fatal(err)
	}
	if err := setupPage.WaitForURL("**/setup"); err != nil {
		t.Fatalf("sin usuarios debe abrir setup: %v", err)
	}
	if err := setupPage.Locator("form input").Nth(0).Fill("jefe"); err != nil {
		t.Fatal(err)
	}
	if err := setupPage.Locator("form input").Nth(1).Fill("password-larga-1"); err != nil {
		t.Fatal(err)
	}
	if err := setupPage.Locator("form input").Nth(2).Fill("password-larga-1"); err != nil {
		t.Fatal(err)
	}
	if err := setupPage.Locator("form button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	if err := setupPage.WaitForURL("**/overview"); err != nil {
		t.Fatalf("setup no inició sesión: %v", err)
	}

	// Nueva página: perfil aislado, sin la cookie de setup.
	loginPage, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loginPage.Goto(server.URL + "/overview"); err != nil {
		t.Fatal(err)
	}
	if err := loginPage.WaitForURL("**/login"); err != nil {
		t.Fatalf("sesión nueva debe pedir login: %v", err)
	}
	if err := loginPage.Locator("form input").Nth(0).Fill("jefe"); err != nil {
		t.Fatal(err)
	}
	if err := loginPage.Locator("form input").Nth(1).Fill("password-larga-1"); err != nil {
		t.Fatal(err)
	}
	if err := loginPage.Locator("form button[type=submit]").Click(); err != nil {
		t.Fatal(err)
	}
	if err := loginPage.WaitForURL("**/overview"); err != nil {
		t.Fatalf("login no abrió el dashboard: %v", err)
	}
}
