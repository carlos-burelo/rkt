package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"rkt-proxy/internal/config"
)

func liveFrom(t *testing.T, body string) *config.Live {
	t.Helper()
	cfg, _, err := config.ImportJSON([]byte(body))
	if err != nil {
		t.Fatalf("config: %v", err)
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
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"5s","rate_limit":{"enabled":false},"routes":[{"hosts":["h.test"],"target":"http://%s"}]}`, addr))
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
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"300ms","rate_limit":{"enabled":false},"routes":[{"hosts":["h.test"],"target":"http://%s"}]}`, addr))
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
	live := liveFrom(t, `{"max_body_mb":1,"rate_limit":{"enabled":false},"routes":[{"hosts":["h.test"],"target":"http://127.0.0.1:9"}]}`)
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

func TestMaxBodyCeroYRateLimit(t *testing.T) {
	live := liveFrom(t, `{"max_body_mb":0,"restart_grace":"0s","rate_limit":{"enabled":true,"requests_per_second":0.001,"burst":1},"routes":[{"hosts":["h.test"],"target":"http://127.0.0.1:9"}]}`)
	h := NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 3<<20)))
	req.Host = "h.test"
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatal("max_body_mb:0 no debe limitar")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Host = "h.test"
	req2.RemoteAddr = "127.0.0.1:1"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("segunda petición = %d, quiere 429", rec2.Code)
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

// TestBodyChunkedExcedeTope: sin Content-Length (chunked) el tope se
// descubre al proxyar; debe responder 413, no 502.
func TestBodyChunkedExcedeTope(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	live := liveFrom(t, fmt.Sprintf(`{"max_body_mb":1,"rate_limit":{"enabled":false},"routes":[{"hosts":["h.test"],"target":"%s"}]}`, backend.URL))
	h := NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil)))

	big := bytes.Repeat([]byte("a"), 2<<20)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(big))
	req.ContentLength = -1 // chunked
	req.Host = "h.test"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked grande = %d, quiere 413 (%s)", rec.Code, rec.Body.String())
	}
}

// TestClienteCanceladoNoEsBackendCaido: si el cliente cancela, el proxy no
// responde 502 ni inventa un error de backend.
func TestClienteCanceladoNoEsBackendCaido(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("tarde"))
	}))
	defer backend.Close()
	live := liveFrom(t, fmt.Sprintf(`{"rate_limit":{"enabled":false},"routes":[{"hosts":["h.test"],"target":"%s"}]}`, backend.URL))
	h := NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	req.Host = "h.test"
	cancel()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusBadGateway {
		t.Fatalf("cliente cancelado no debe ser 502: %s", rec.Body.String())
	}
}

// TestForwardedProtoYHost: TLS y X-Forwarded-Proto previo se propagan.
func TestForwardedProtoYHost(t *testing.T) {
	var got http.Header
	var gotHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotHost = r.Host
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	live := liveFrom(t, fmt.Sprintf(`{"rate_limit":{"enabled":false},"restart_grace":"0s","routes":[{"hosts":["public.test"],"target":"%s"}]}`, backend.URL))
	h := NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "public.test"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy = %d", rec.Code)
	}
	if got.Get("X-Forwarded-Proto") != "https, https" {
		t.Fatalf("proto = %q", got.Get("X-Forwarded-Proto"))
	}
	if got.Get("X-Forwarded-Host") != "public.test" {
		t.Fatalf("xfh = %q", got.Get("X-Forwarded-Host"))
	}
	if gotHost == "public.test" {
		t.Fatal("Host al backend no debe ser el público")
	}
}

func TestWebSocketUpgrade(t *testing.T) {
	backend := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		_, _ = io.Copy(ws, ws)
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[{"hosts":["ws.test"],"target":"ws://%s"}]}`, addr))
	proxySrv := httptest.NewServer(NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer proxySrv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(proxySrv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := websocket.NewConfig("ws://ws.test/", "http://ws.test/")
	ws, err := websocket.NewClient(cfg, conn)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	defer ws.Close()
	if _, err := ws.Write([]byte("hola")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, err := ws.Read(buf)
	if err != nil || string(buf[:n]) != "hola" {
		t.Fatalf("eco = %q %v", buf[:n], err)
	}
}

func TestSSEFlushInmediato(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: uno\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(400 * time.Millisecond)
		fmt.Fprint(w, "data: dos\n\n")
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[{"hosts":["sse.test"],"target":"http://%s"}]}`, addr))
	proxySrv := httptest.NewServer(NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer proxySrv.Close()

	req, _ := http.NewRequest("GET", proxySrv.URL, nil)
	req.Host = "sse.test"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	start := time.Now()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || !strings.Contains(line, "uno") {
		t.Fatalf("primera línea = %q %v", line, err)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatalf("SSE retrasado: %v", time.Since(start))
	}
}

func TestHTTP2EnProxyTLS(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[{"hosts":["h2.test"],"target":"http://%s"}]}`, addr))
	srv := httptest.NewUnstartedServer(NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil))))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		ForceAttemptHTTP2: true,
	}
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Host = "h2.test"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("protocolo = %s", resp.Proto)
	}
}

func TestWSSUpgrade(t *testing.T) {
	backend := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		_, _ = io.Copy(ws, ws)
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[{"hosts":["wss.test"],"target":"ws://%s"}]}`, addr))
	srv := httptest.NewUnstartedServer(NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil))))
	srv.StartTLS()
	defer srv.Close()

	raw, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	tconn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, ServerName: "wss.test"}) //nolint:gosec
	cfg, _ := websocket.NewConfig("wss://wss.test/", "https://wss.test/")
	ws, err := websocket.NewClient(cfg, tconn)
	if err != nil {
		t.Fatalf("handshake wss: %v", err)
	}
	defer ws.Close()
	if _, err := ws.Write([]byte("eco")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, err := ws.Read(buf)
	if err != nil || string(buf[:n]) != "eco" {
		t.Fatalf("eco wss = %q %v", buf[:n], err)
	}
}

func TestWebSocketConcurrente(t *testing.T) {
	backend := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		_, _ = io.Copy(ws, ws)
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[{"hosts":["ws.test"],"target":"ws://%s"}]}`, addr))
	proxySrv := httptest.NewServer(NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer proxySrv.Close()

	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(n int) {
			conn, err := net.Dial("tcp", strings.TrimPrefix(proxySrv.URL, "http://"))
			if err != nil {
				errs <- err
				return
			}
			cfg, _ := websocket.NewConfig("ws://ws.test/", "http://ws.test/")
			ws, err := websocket.NewClient(cfg, conn)
			if err != nil {
				errs <- err
				return
			}
			defer ws.Close()
			msg := fmt.Sprintf("m%d", n)
			if _, err := ws.Write([]byte(msg)); err != nil {
				errs <- err
				return
			}
			buf := make([]byte, 8)
			got, err := ws.Read(buf)
			if err != nil || string(buf[:got]) != msg {
				errs <- fmt.Errorf("eco %q: %v", buf[:got], err)
				return
			}
			errs <- nil
		}(i)
	}
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}
