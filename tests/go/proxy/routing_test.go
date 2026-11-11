package proxy

// Estrategia por path a nivel HTTP: el prefijo se recorta antes de
// proxear; el prefijo parcial no caza; host desconocido da 404.

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func echoBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s", r.Host, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProxyPathRecorta(t *testing.T) {
	back := echoBackend(t)
	target := strings.TrimPrefix(back.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[
		{"hosts":["h.test"],"path":"/app","target":"http://%s"},
		{"hosts":["h.test"],"target":"http://%s"}
	]}`, target, target))
	h := NewHandler(live, slog.New(slog.NewTextHandler(io.Discard, nil)))

	get := func(host, path string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("GET", "http://"+host+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		b, _ := io.ReadAll(rec.Result().Body)
		return rec.Code, string(b)
	}
	if code, body := get("h.test", "/app/hola"); code != 200 || body != target+"|/hola" {
		t.Fatalf("recorte: %d %q", code, body)
	}
	if code, body := get("h.test", "/app"); code != 200 || body != target+"|/" {
		t.Fatalf("prefijo exacto: %d %q", code, body)
	}
	// /app2 no es subruta de /app: cae en la ruta pelada del host.
	if code, body := get("h.test", "/app2"); code != 200 || body != target+"|/app2" {
		t.Fatalf("fallback host: %d %q", code, body)
	}
	if code, _ := get("otro.test", "/app/hola"); code != 404 {
		t.Fatalf("host desconocido: %d", code)
	}
}
