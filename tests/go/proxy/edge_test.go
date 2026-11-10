package proxy

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

func TestCompressible(t *testing.T) {
	cases := []struct {
		ct     string
		status int
		want   bool
	}{
		{"text/html; charset=utf-8", 200, true},
		{"application/json", 200, true},
		{"application/javascript", 200, true},
		{"application/xml", 200, true},
		{"application/xhtml+xml", 200, true},
		{"image/svg+xml", 200, true},
		{"image/png", 200, false},
		{"", 200, false},
		{"text/plain", http.StatusNoContent, false},
		{"text/plain", http.StatusNotModified, false},
		{"text/plain", http.StatusPartialContent, false},
	}
	for _, c := range cases {
		if got := compressible(c.ct, c.status); got != c.want {
			t.Fatalf("compressible(%q,%d)=%v", c.ct, c.status, got)
		}
	}
}

func TestGzipResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	g := &gzipResponse{ResponseWriter: rec}
	g.Header().Set("Content-Type", "text/plain")
	if _, err := g.Write([]byte("hola")); err != nil {
		t.Fatal(err)
	}
	g.Flush()
	g.Close()
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("sin gzip: %v", rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if string(body) != "hola" {
		t.Fatalf("body = %q", body)
	}
	if g.Unwrap() != rec {
		t.Fatal("Unwrap")
	}

	// No comprime si ya hay encoding o rango.
	for _, h := range []string{"Content-Encoding", "Content-Range"} {
		rec := httptest.NewRecorder()
		g := &gzipResponse{ResponseWriter: rec}
		g.Header().Set("Content-Type", "text/plain")
		g.Header().Set(h, "x")
		g.WriteHeader(http.StatusOK)
		g.Write([]byte("x"))
		if rec.Header().Get("Content-Encoding") == "gzip" {
			t.Fatalf("%s no debe comprimir", h)
		}
	}
	// Decidir dos veces no duplica.
	rec = httptest.NewRecorder()
	g = &gzipResponse{ResponseWriter: rec}
	g.Header().Set("Content-Type", "text/plain")
	g.decide(200)
	g.decide(200)
	g.Close()
}

func TestCompressMiddleware(t *testing.T) {
	h := Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("contenido"))
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("debe comprimir")
	}
	// Upgrade y sin accept-encoding pasan sin envolver.
	for _, setup := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Upgrade", "websocket") },
		func(r *http.Request) { r.Header.Del("Accept-Encoding") },
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		setup(req)
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Content-Encoding") == "gzip" && req.Header.Get("Upgrade") == "" {
			t.Fatal("no debe comprimir")
		}
	}
}

func TestEdgeHeadersYRequestID(t *testing.T) {
	h := RequestID(EdgeHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := httptest.NewRequest("GET", "/", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Strict-Transport-Security") == "" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		rec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("cabeceras: %v", rec.Header())
	}
	// Id largo se reemplaza por uno generado.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", strings.Repeat("a", 100))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-Id") == strings.Repeat("a", 100) {
		t.Fatal("id largo debe regenerarse")
	}
	// Sin TLS no hay HSTS.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS solo con TLS")
	}
}

func TestIPAllowed(t *testing.T) {
	if ipAllowed("nope", []string{"10.0.0.0/8"}) || !ipAllowed("10.1.2.3", []string{"10.0.0.0/8"}) {
		t.Fatal("ipAllowed mal")
	}
	if ipAllowed("10.1.2.3", []string{"bad"}) {
		t.Fatal("CIDR inválido no debe permitir")
	}
}

func TestIsDialErr(t *testing.T) {
	if !isDialErr(&net.OpError{Op: "dial", Err: errors.New("x")}) {
		t.Fatal("dial error")
	}
	if isDialErr(errors.New("otro")) {
		t.Fatal("error no dial")
	}
}

type fakeRT struct {
	calls int
	errs  int
	err   error
	resp  *http.Response
}

func (f *fakeRT) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.errs {
		return nil, f.err
	}
	return f.resp, nil
}

func TestGraceTransport(t *testing.T) {
	cfg := config.Default()
	cfg.RestartGrace = "30ms"
	if err := cfg.Finalize(); err != nil {
		t.Fatal(err)
	}
	live := config.NewLive(cfg)
	dialErr := &net.OpError{Op: "dial", Err: errors.New("refused")}
	base := &fakeRT{errs: 1, err: dialErr, resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tr := &graceTransport{base: base, live: live, log: log}
	req := httptest.NewRequest("GET", "http://127.0.0.1:1/", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil || resp.StatusCode != 200 || base.calls < 2 {
		t.Fatalf("reintento: %v calls=%d", err, base.calls)
	}
	// Error no-dial no reintenta.
	base = &fakeRT{errs: 9, err: errors.New("otro")}
	if _, err := (&graceTransport{base: base, live: live, log: log}).RoundTrip(req); err == nil || base.calls != 1 {
		t.Fatal("no-dial no debe reintentar")
	}
	// Contexto cancelado corta.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req = httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	base = &fakeRT{errs: 9, err: dialErr}
	if _, err := (&graceTransport{base: base, live: live, log: log}).RoundTrip(req); err == nil {
		t.Fatal("contexto cancelado")
	}
	// Body con GetBody disponible se rebobina; sin GetBody falla.
	req = httptest.NewRequest("POST", "/", strings.NewReader("x"))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("x")), nil }
	base = &fakeRT{errs: 1, err: dialErr, resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}}
	if _, err := (&graceTransport{base: base, live: live, log: log}).RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader("x")))
	base = &fakeRT{errs: 9, err: dialErr}
	if _, err := (&graceTransport{base: base, live: live, log: log}).RoundTrip(req); err == nil {
		t.Fatal("body sin GetBody no debe reintentar")
	}
	// GetBody con error.
	req = httptest.NewRequest("POST", "/", strings.NewReader("x"))
	req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("gar") }
	base = &fakeRT{errs: 9, err: dialErr}
	if _, err := (&graceTransport{base: base, live: live, log: log}).RoundTrip(req); err == nil {
		t.Fatal("GetBody con error")
	}
	_ = time.Second
}
