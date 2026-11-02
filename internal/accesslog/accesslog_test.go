package accesslog

import (
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
