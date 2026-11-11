package waf

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
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

func TestDenyMethods(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, nil) // defaults traen TRACE/TRACK/CONNECT

	for _, m := range []string{"TRACE", "TRACK", "CONNECT"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest(m, "/", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, quiere 403", m, rec.Code)
		}
	}
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest(m, "/", nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s: got %d, quiere paso", m, rec.Code)
		}
	}
}

func TestDenyEscapado(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, []string{"/.env", "/s%20dir/*"})

	// Patrón con escape: el path decodificado no matchea, el escapado sí.
	u, err := url.Parse("http://a.test/s%20dir/x")
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{Method: "GET", URL: u, Host: "a.test", Header: http.Header{}}
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("escapado: got %d, quiere 403", rec.Code)
	}
	// Sin escape no hay falso positivo: otra carpeta con espacio pasa.
	u2, _ := url.Parse("http://a.test/otra%20dir/x")
	req2 := &http.Request{Method: "GET", URL: u2, Host: "a.test", Header: http.Header{}}
	rec2 := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTeapot {
		t.Fatalf("otra carpeta: got %d, quiere paso", rec2.Code)
	}
}

func TestRecentRing(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ringMu.Lock()
	ring = nil
	ringMu.Unlock()

	live := liveWith(t, []string{"/.env"})
	req := httptest.NewRequest("TRACE", "/.env", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	req.Host = "a.test"
	Middleware(live, next, log).ServeHTTP(httptest.NewRecorder(), req)

	recent := Recent()
	if len(recent) != 1 {
		t.Fatalf("bloqueos = %d, quiere 1", len(recent))
	}
	b := recent[0]
	if b.IP != "9.9.9.9" || b.Host != "a.test" || b.Method != "TRACE" || b.Path != "/.env" || b.Rule == "" || b.At == "" {
		t.Fatalf("bloqueo mal registrado: %+v", b)
	}
	// el método manda sobre el path en el motivo
	if !strings.Contains(b.Rule, "TRACE") {
		t.Fatalf("motivo = %q, quiere el método", b.Rule)
	}

	// tope del anillo: solo sobreviven los últimos 50
	ringMu.Lock()
	ring = nil
	for i := 0; i < 60; i++ {
		ring = append(ring, Block{Path: fmt.Sprintf("/%d", i)})
	}
	ringMu.Unlock()
	req = httptest.NewRequest("GET", "/.env", nil)
	Middleware(live, next, log).ServeHTTP(httptest.NewRecorder(), req)
	if got := len(Recent()); got != maxBlocks {
		t.Fatalf("anillo = %d, quiere %d", got, maxBlocks)
	}
}
