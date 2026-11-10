package waf

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"rkt-proxy/internal/config"
)

func liveWith(t *testing.T, deny []string) *config.Live {
	t.Helper()
	cfg := config.Default()
	cfg.Routes = []config.Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"}}
	if deny != nil {
		cfg.DenyPaths = deny
	}
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return config.NewLive(cfg)
}

func TestDeny(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, []string{"/.env", "/php-cgi*", "/php-cgi/*", "/.git/*"})

	for _, p := range []string{"/.env", "/php-cgi", "/php-cgi/php-cgi.exe", "/.git/config"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, quiere 403", p, rec.Code)
		}
	}
	for _, p := range []string{"/", "/login", "/env", "/phpinfo", "/api/health", "/_next/static/x.js"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s: got %d, quiere paso", p, rec.Code)
		}
	}
}

func TestDenyDepthAndHeaders(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, nil) // defaults

	// Profundidad: el basename manda.
	for _, p := range []string{
		"/.env", "/.env.production", "/api/.env",
		"/x.php", "/api/x.php", "/wp-admin/setup-config.php",
		"/backup.sql", "/db/dump.sql", "/config.xml", "/database.sqlite",
		"/.ssh/id_rsa", "/_vti_pvt/service.pwd", "/docker-compose.yml",
		"/.git/config", "/actuator/health", "/wp-login.php",
	} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, quiere 403", p, rec.Code)
		}
	}
	// Cabecera interna de Next desde fuera = ataque (CVE-2025-29927).
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Middleware-Subrequest", "middleware")
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("header interno: got %d, quiere 403", rec.Code)
	}
	// Cabeceras normales pasan.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc")
	rec = httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("header normal: got %d, quiere paso", rec.Code)
	}
}

func TestDefaultsDenyCommon(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, nil) // defaults
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", "/.env", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("default debe denegar /.env, got %d", rec.Code)
	}
}

func TestValidPattern(t *testing.T) {
	if !ValidPattern("/a/*.php") || ValidPattern("/a/[") {
		t.Fatal("validación de glob mal")
	}
}
