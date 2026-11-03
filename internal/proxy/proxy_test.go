package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

func liveFrom(t *testing.T, body string) *config.Live {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-proxy-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return config.NewLive(cfg)
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestGraceWaitsForBackend(t *testing.T) {
	addr := freePort(t)
	live := liveFrom(t, fmt.Sprintf(`
restart_grace: 5s
rate_limit:
  enabled: false
routes:
  - hosts: ["h.test"]
    target: "http://%s"
`, addr))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(live, log)

	// backend aparece a los 600ms; con gracia 5s debe dar 200
	started := make(chan *http.Server, 1)
	time.AfterFunc(600*time.Millisecond, func() {
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		})}
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		started <- srv
		go srv.Serve(l)
	})
	defer func() {
		select {
		case srv := <-started:
			_ = srv.Close()
		default:
		}
	}()
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "h.test"
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("con backend tardío: got %d, quiere 200", rec.Code)
	}
}

func TestGraceExpires502(t *testing.T) {
	addr := freePort(t) // nadie escucha: puerto libre tras cerrar
	live := liveFrom(t, fmt.Sprintf(`
restart_grace: 300ms
rate_limit:
  enabled: false
routes:
  - hosts: ["h.test"]
    target: "http://%s"
`, addr))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(live, log)
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "h.test"
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("sin backend: got %d, quiere 502", rec.Code)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("502 debe llegar tras la gracia, no colgarse")
	}
}

func TestMaxBody(t *testing.T) {
	live := liveFrom(t, `
max_body_mb: 1
rate_limit:
  enabled: false
routes:
  - hosts: ["h.test"]
    target: "http://127.0.0.1:9"
`)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(live, log)

	big := httptest.NewRequest("POST", "/", strings.NewReader("x"))
	big.Host = "h.test"
	big.RemoteAddr = "127.0.0.1:1"
	big.ContentLength = 2 << 20
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("2MB con tope 1MB: got %d, quiere 413", rec.Code)
	}

	small := httptest.NewRequest("POST", "/", strings.NewReader("x"))
	small.Host = "otra.test"
	small.RemoteAddr = "127.0.0.1:1"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, small)
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatal("request chico no debe dar 413")
	}
}

func TestRecovery(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	panicer := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom-test")
	})
	rec := httptest.NewRecorder()
	recovery(log, panicer).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic: got %d, quiere 500", rec.Code)
	}
}

// TestRecoveryAbortHandler: si el cliente corta la conexión el proxy paniquea
// con http.ErrAbortHandler; eso no es un 500 ni debe escribir cabeceras.
func TestRecoveryAbortHandler(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	abort := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		panic(http.ErrAbortHandler)
	})
	rec := httptest.NewRecorder()
	recovery(log, abort).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("abort: got %d, quiere 200 ya enviado", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("abort no debe escribir body: %q", body)
	}
}
