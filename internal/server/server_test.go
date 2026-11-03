package server

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"rkt-proxy/internal/config"
)

func testLive(t *testing.T, redirect bool) *config.Live {
	t.Helper()
	cfg := config.Default()
	cfg.ListenHTTP = "127.0.0.1:0"
	cfg.ListenHTTPS = "127.0.0.1:0"
	cfg.RedirectHTTPToHTTPS = redirect
	if err := cfg.Finalize(); err != nil {
		t.Fatal(err)
	}
	return config.NewLive(cfg)
}

func okHandler(mark string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Mark", mark)
		_, _ = w.Write([]byte(mark))
	})
}

// TestNewHTTPEnrutaControlChallengeYBackend: orden de precedencia del listener 80.
func TestNewHTTPEnrutaControlChallengeYBackend(t *testing.T) {
	live := testLive(t, false)
	srv := NewHTTP(live.Get(), live, okHandler("challenge"), okHandler("backend"), okHandler("control"), nil)
	h := srv.Handler

	cases := []struct{ path, mark string }{
		{"/_rkt/hook/github", "control"},
		{"/.well-known/acme-challenge/x", "challenge"},
		{"/cualquier-cosa", "backend"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Header().Get("X-Mark") != c.mark {
			t.Fatalf("%s -> %q, quiere %q", c.path, rec.Header().Get("X-Mark"), c.mark)
		}
	}
}

// TestNewHTTPRedirige: con redirect on, todo va a https conservando path+query.
func TestNewHTTPRedirige(t *testing.T) {
	live := testLive(t, true)
	srv := NewHTTP(live.Get(), live, okHandler("challenge"), okHandler("backend"), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "http://app.test/x?y=1", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("redirect = %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://app.test/x?y=1" {
		t.Fatalf("Location = %q", loc)
	}
}

// TestNewHTTPControlNilYChallengeNil: sin control ni challenge no panic.
func TestNewHTTPControlNilYChallengeNil(t *testing.T) {
	live := testLive(t, false)
	srv := NewHTTP(live.Get(), live, nil, okHandler("backend"), nil, nil)
	for _, p := range []string{"/_rkt/x", "/.well-known/acme-challenge/x"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s = %d, quiere 404", p, rec.Code)
		}
	}
}

// TestNewHTTPSUsaGetCert: el certificado lo elige el proveedor inyectado.
func TestNewHTTPSUsaGetCert(t *testing.T) {
	live := testLive(t, true)
	called := false
	srv := NewHTTPS(live.Get(), func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		called = true
		return nil, errors.New("sin cert en test")
	}, okHandler("backend"), nil, nil)
	if srv.TLSConfig == nil || srv.TLSConfig.GetCertificate == nil {
		t.Fatal("TLSConfig.GetCertificate debe estar cableado")
	}
	if _, err := srv.TLSConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "x.test"}); err == nil {
		t.Fatal("el error del proveedor debe propagarse")
	}
	if !called {
		t.Fatal("getCert no se llamó")
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("X-Mark") != "backend" {
		t.Fatalf("https backend mal: %v", rec.Header())
	}
}

// TestNewHTTPSControl: el plano /_rkt/* va al control, no al backend.
func TestNewHTTPSControl(t *testing.T) {
	live := testLive(t, true)
	srv := NewHTTPS(live.Get(), func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, nil },
		okHandler("backend"), okHandler("control"), nil)
	for path, want := range map[string]string{"/_rkt/hook": "control", "/app": "backend"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("X-Mark"); got != want {
			t.Fatalf("%s = %q, quiere %q", path, got, want)
		}
	}
	// control nil => 404 en /_rkt
	srv = NewHTTPS(live.Get(), func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return nil, nil },
		okHandler("backend"), nil, nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/x", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("control nil = %d", rec.Code)
	}
}

// TestHealthzPayloadNoSeUsaAqui evita dependencias: el JSON del healthz del
// listener 80 no existe; solo verificamos que el backend responde JSON propio.
func TestControlRespetaMetodo(t *testing.T) {
	live := testLive(t, false)
	srv := NewHTTP(live.Get(), live, nil, okHandler("backend"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}), nil)
	req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("control = %d", rec.Code)
	}
}
