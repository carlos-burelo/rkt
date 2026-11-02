package ratelimit

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
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
