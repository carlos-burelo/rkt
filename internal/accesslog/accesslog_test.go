package accesslog

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMiddlewareLine(t *testing.T) {
	dir := t.TempDir()
	l, err := New(filepath.Join(dir, "access.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hola"))
	})
	req := httptest.NewRequest("GET", "/x?q=1", nil)
	req.RemoteAddr = "1.2.3.4:567"
	req.Host = "h.test"
	req.Header.Set("User-Agent", "curl-test")
	rec := httptest.NewRecorder()
	l.Middleware(next).ServeHTTP(rec, req)

	data, _ := os.ReadFile(filepath.Join(dir, "access.log"))
	line := string(data)
	for _, want := range []string{"1.2.3.4", `"GET /x?q=1 HTTP/1.1"`, " 200 ", " 4 ", "h.test", "curl-test"} {
		if !strings.Contains(line, want) {
			t.Fatalf("línea sin %q: %q", want, line)
		}
	}
}

func TestCapturesErrorStatus(t *testing.T) {
	dir := t.TempDir()
	l, _ := New(filepath.Join(dir, "a.log"))
	defer l.Close()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusNotFound)
	})
	rec := httptest.NewRecorder()
	l.Middleware(next).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	data, _ := os.ReadFile(filepath.Join(dir, "a.log"))
	if !strings.Contains(string(data), " 404 ") {
		t.Fatalf("debió registrar 404: %q", data)
	}
}

// TestRecorderFlush: el writer envuelto debe seguir exponiendo Flush al
// ResponseController (streaming/SSE/rangos de video).
func TestRecorderFlush(t *testing.T) {
	base := newFlushSpy()
	r := &recorder{ResponseWriter: base}
	if err := http.NewResponseController(r).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if !base.flushed {
		t.Fatal("el Flush no llegó al writer real")
	}
}

// TestRecorderHijack: WebSockets necesitan Hijack a través del middleware.
func TestRecorderHijack(t *testing.T) {
	base := &hijackSpy{ResponseWriter: httptest.NewRecorder()}
	r := &recorder{ResponseWriter: base}
	conn, _, err := http.NewResponseController(r).Hijack()
	if err != nil {
		t.Fatalf("Hijack: %v", err)
	}
	_ = conn.Close()
	if !base.hijacked {
		t.Fatal("el Hijack no llegó al writer real")
	}
}

type hijackSpy struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackSpy) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	c1, c2 := net.Pipe()
	return c1, bufio.NewReadWriter(bufio.NewReader(c2), bufio.NewWriter(c2)), nil
}

// TestWriteHeaderUnaVez: una cabecera final repetida se ignora en vez de
// provocar "superfluous response.WriteHeader call".
func TestWriteHeaderUnaVez(t *testing.T) {
	base := httptest.NewRecorder()
	r := &recorder{ResponseWriter: base}
	r.WriteHeader(http.StatusOK)
	r.WriteHeader(http.StatusInternalServerError)
	if base.Code != http.StatusOK {
		t.Fatalf("código = %d, quiere 200", base.Code)
	}
	if r.status != http.StatusOK {
		t.Fatalf("status registrado = %d, quiere 200", r.status)
	}
}

type flushSpy struct {
	http.ResponseWriter
	flushed bool
}

func newFlushSpy() *flushSpy { return &flushSpy{ResponseWriter: httptest.NewRecorder()} }

func (f *flushSpy) Flush() { f.flushed = true }

func TestNewRotaYErrores(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	if err := os.WriteFile(path, make([]byte, maxBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Close()
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotación: %v", err)
	}

	// dir padre que es un archivo => error de MkdirAll
	f := filepath.Join(dir, "archivo")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(f, "sub", "access.log")); err == nil {
		t.Fatal("MkdirAll sobre archivo debe fallar")
	}

	// el path es un directorio => OpenFile falla
	asDir := filepath.Join(dir, "dir.log")
	if err := os.Mkdir(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := New(asDir); err == nil {
		t.Fatal("abrir un directorio como log debe fallar")
	}
}

func TestMiddlewareSinEscribirYRepetido(t *testing.T) {
	dir := t.TempDir()
	l, err := New(filepath.Join(dir, "a.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	// handler que no escribe nada: se registra 200
	l.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/vacio", nil),
	)
	// handler que escribe cabecera dos veces: la segunda se ignora
	l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusInternalServerError)
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/doble", nil))

	data, _ := os.ReadFile(filepath.Join(dir, "a.log"))
	text := string(data)
	if !strings.Contains(text, " 200 ") || !strings.Contains(text, "/vacio") {
		t.Fatalf("sin escritura debe ser 200: %q", text)
	}
	if !strings.Contains(text, " 201 ") || strings.Contains(text, " 500 ") {
		t.Fatalf("cabecera repetida mal: %q", text)
	}
}
