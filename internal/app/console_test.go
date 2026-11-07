package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"rkt-proxy/internal/config"
)

func TestDashboardGate(t *testing.T) {
	cfg := config.Default()
	cfg.Dashboard.Host = "rkt.test"
	if err := cfg.Finalize(); err != nil {
		t.Fatal(err)
	}
	live := config.NewLive(cfg)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("api")) })
	dash := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("dash")) })
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("app")) })
	h := dashboardGate(live, api, dash, next)

	cases := []struct{ host, path, want string }{
		{"rkt.test", "/api/v1/status", "api"},
		{"RKT.TEST:443", "/", "dash"},
		{"app.test", "/", "app"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Body.String() != c.want {
			t.Fatalf("%s %s = %q, quiere %q", c.host, c.path, rec.Body.String(), c.want)
		}
	}
}

func TestConsoleOnly(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("api")) })
	dash := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("dash")) })
	h := consoleOnly(api, dash)
	for path, want := range map[string]string{
		"/api/v1/apps": "api",
		"/":            "dash",
		"/healthz":     "dash", // nunca control en el listener público
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Body.String() != want {
			t.Fatalf("%s = %q", path, rec.Body.String())
		}
	}
}

func TestPortOfYLoopbackRemote(t *testing.T) {
	if portOf("0.0.0.0:8090") != "8090" || portOf("sin-puerto") != "" {
		t.Fatal("portOf mal")
	}
	for addr, want := range map[string]bool{
		"127.0.0.1:123": true,
		"[::1]:123":     true,
		"10.0.0.2:123":  false,
		"localhost:123": true,
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = addr
		if got := isLoopbackRemote(r); got != want {
			t.Fatalf("%s = %v", addr, got)
		}
	}
}

func TestDashboardReturnURL(t *testing.T) {
	cfg := config.Default()
	cfg.Dashboard.Host = "rkt.test"
	cfg.ListenHTTPS = ":443"
	if got := dashboardReturnURL(cfg); got != "https://rkt.test/settings?github=ok" {
		t.Fatalf("https = %q", got)
	}
	cfg.ListenHTTPS = ""
	cfg.LocalCA = false
	if got := dashboardReturnURL(cfg); got != "http://rkt.test/settings?github=ok" {
		t.Fatalf("http = %q", got)
	}
	cfg.Dashboard.Host = ""
	if got := dashboardReturnURL(cfg); got == "" {
		t.Fatal("loopback por defecto")
	}
}
