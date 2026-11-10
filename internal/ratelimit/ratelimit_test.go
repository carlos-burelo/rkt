package ratelimit

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestAllowBurstThenDeny(t *testing.T) {
	l := New(1, 2) // 1 rps, burst 2
	defer l.Stop()
	if !l.Allow("1.1.1.1") || !l.Allow("1.1.1.1") {
		t.Fatal("burst debe pasar")
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("tercera debe denegarse")
	}
	if !l.Allow("2.2.2.2") {
		t.Fatal("otra IP con bucket propio debe pasar")
	}
}

func TestRefill(t *testing.T) {
	l := New(10, 1)
	defer l.Stop()
	if !l.Allow("1.1.1.1") {
		t.Fatal("primera pasa")
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("sin token debe fallar")
	}
	time.Sleep(200 * time.Millisecond) // 10rps = 1 token / 100ms
	if !l.Allow("1.1.1.1") {
		t.Fatal("tras refill debe pasar")
	}
}

func TestMiddleware429(t *testing.T) {
	l := New(1, 1)
	defer l.Stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := l.Middleware(next, log)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("primera: got %d", rec.Code)
	}
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "9.9.9.9:1234"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda: got %d, quiere 429", rec2.Code)
	}
}

func TestMiddlewareHostNoEvade(t *testing.T) {
	l := New(1, 1)
	defer l.Stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := l.Middleware(next, log)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	req.Host = "a.test"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatal("primera debe pasar")
	}
	// misma IP, otro Host: el bucket es por IP, no por Host
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "9.9.9.9:1234"
	req2.Host = "b.test"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("rotar Host no debe evadir: got %d, quiere 429", rec2.Code)
	}
}

func TestAllowlistYRetryAfter(t *testing.T) {
	l := New(1, 1, "10.0.0.0/8", "bad-cidr")
	defer l.Stop()
	if l.ipAllowed("nope") {
		t.Fatal("IP inválida no debe permitirse")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := l.Middleware(next, log)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.1.2.3:123"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("allowlist debe saltar el límite: %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "9.9.9.9:123"
	req.Host = "a.test:443"
	h.ServeHTTP(httptest.NewRecorder(), req)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("429 con Retry-After: %d %v", rec.Code, rec.Header())
	}
}

func TestSweepYStop(t *testing.T) {
	l := New(100, 10)
	defer l.Stop()
	l.Allow("viejo.test")
	l.Allow("activo.test")
	l.mu.Lock()
	l.clients["viejo.test"].lastSeen = time.Now().Add(-2 * idleTTL)
	l.mu.Unlock()
	l.sweep(time.Now())
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.clients["viejo.test"]; ok {
		t.Fatal("bucket idle debe salir")
	}
	if _, ok := l.clients["activo.test"]; !ok {
		t.Fatal("bucket activo debe quedarse")
	}
	l.Stop() // idempotente
	l.Stop()
}

func TestTickerLimpiaSolo(t *testing.T) {
	old := atomic.LoadInt64(&sweepEvery)
	atomic.StoreInt64(&sweepEvery, int64(10*time.Millisecond))
	t.Cleanup(func() { atomic.StoreInt64(&sweepEvery, old) })
	l := New(100, 10)
	defer l.Stop()
	l.Allow("viejo.test")
	l.mu.Lock()
	l.clients["viejo.test"].lastSeen = time.Now().Add(-2 * idleTTL)
	l.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		l.mu.Lock()
		_, ok := l.clients["viejo.test"]
		l.mu.Unlock()
		if !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("el ticker no limpió el bucket idle")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
