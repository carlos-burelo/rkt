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
	f, err := os.CreateTemp("", "rkt-waf-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())
	body := "routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n"
	if deny != nil {
		body = "deny_paths:\n"
		for _, d := range deny {
			body += "  - \"" + d + "\"\n"
		}
		body += "routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n"
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
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
	for _, p := range []string{"/", "/login", "/env", "/phpinfo"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s: got %d, quiere paso", p, rec.Code)
		}
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
