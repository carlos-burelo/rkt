package staticweb

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRootExigeSitioCompilado(t *testing.T) {
	if err := validateRoot(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Fatal("quiero error sin directorio")
	}
	empty := t.TempDir()
	if err := validateRoot(empty); err == nil {
		t.Fatal("quiero error sin index.html")
	}
	if err := os.WriteFile(filepath.Join(empty, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateRoot(empty); err != nil {
		t.Fatalf("sitio compilado: %v", err)
	}
}

func TestHandlerFallbackY404(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>spa</html>"), 0o644)
	h := Handler(root)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ruta-spa", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>spa</html>" {
		t.Fatalf("fallback = %d %q", rec.Code, rec.Body.String())
	}

	_ = os.WriteFile(filepath.Join(root, "404.html"), []byte("<html>no</html>"), 0o644)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no-existe", nil))
	if rec.Code != http.StatusNotFound || rec.Body.String() != "<html>no</html>" {
		t.Fatalf("404 = %d %q", rec.Code, rec.Body.String())
	}
}

// TestAssetFaltanteNoDevuelveIndex: pedir /x.js inexistente no debe servir
// el index (el navegador recibiría HTML como JS).
func TestAssetFaltanteNoDevuelveIndex(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>spa</html>"), 0o644)
	h := Handler(root)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no-existe.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("asset faltante = %d, quiere 404 (%q)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "spa") {
		t.Fatal("asset faltante no debe devolver index")
	}
}

// TestHeadersDeCacheYTipos: hashed assets inmutables, html sin cache y MIME
// correcto para formatos modernos.
func TestHeadersDeCacheYTipos(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>x</html>"), 0o644)
	astro := filepath.Join(root, "_astro")
	_ = os.MkdirAll(astro, 0o755)
	_ = os.WriteFile(filepath.Join(astro, "app.abc123.js"), []byte("console.log(1)"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "foto.avif"), []byte("avif"), 0o644)
	h := Handler(root)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_astro/app.abc123.js", nil))
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("cache de asset hasheado = %q", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("content-type js = %q", ct)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/foto.avif", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "image/avif" {
		t.Fatalf("content-type avif = %q", ct)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("cache de html = %q", cc)
	}
}

func TestServeErroresYListener(t *testing.T) {
	// raíz inválida: error, sin matar el proceso
	t.Setenv("PORT", "0")
	if err := Serve(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Fatal("raíz inválida debe fallar")
	}
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>ok</html>"), 0o644)
	t.Setenv("PORT", "99999999")
	if err := Serve(root); err == nil {
		t.Fatal("puerto inválido debe fallar")
	}
	// PORT vacío usa 3000 (se corta antes de listen por raíz inválida)
	t.Setenv("PORT", "")
	if err := Serve(filepath.Join(root, "nope")); err == nil {
		t.Fatal("raíz inválida con PORT vacío debe fallar")
	}
	// Serve feliz con listener inyectado
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	old := listen
	listen = func(string, string) (net.Listener, error) { return ln, nil }
	t.Cleanup(func() { listen = old; _ = ln.Close() })
	t.Setenv("PORT", "0")
	done := make(chan error, 1)
	go func() { done <- Serve(root) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("GET Serve: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	_ = ln.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Serve no terminó al cerrar listener")
	}

	// ServeOn real: responde y termina al cerrar el listener
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done2 := make(chan error, 1)
	go func() { done2 <- ServeOn(ln2, root) }()
	resp, err = http.Get("http://" + ln2.Addr().String() + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ServeOn = %d", resp.StatusCode)
	}
	_ = ln2.Close()
	select {
	case <-done2:
	case <-time.After(3 * time.Second):
		t.Fatal("ServeOn no terminó al cerrar listener")
	}
	// ServeOn con raíz inválida también falla
	if err := ServeOn(ln2, filepath.Join(root, "nope")); err == nil {
		t.Fatal("ServeOn raíz inválida debe fallar")
	}
}

func TestCacheForYDirIndex(t *testing.T) {
	cases := map[string]string{
		"/_astro/a.js":       "public, max-age=31536000, immutable",
		"/assets/a.css":      "public, max-age=31536000, immutable",
		"/static/a.png":      "public, max-age=31536000, immutable",
		"/_next/static/a.js": "public, max-age=31536000, immutable",
		"/":                  "no-cache",
		"/index.html":        "no-cache",
		"/foto.jpg":          "public, max-age=3600",
	}
	for path, want := range cases {
		if got := cacheFor(path, strings.ToLower(filepath.Ext(path))); got != want {
			t.Fatalf("cacheFor(%q) = %q, quiere %q", path, got, want)
		}
	}
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>raiz</html>"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "404.html"), []byte("<html>no</html>"), 0o644)
	sub := filepath.Join(root, "sub")
	_ = os.Mkdir(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "index.html"), []byte("<html>sub</html>"), 0o644)
	h := Handler(root)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sub/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sub") {
		t.Fatalf("dir index = %d %q", rec.Code, rec.Body.String())
	}
	// asset faltante con 404.html custom
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope.css", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no") {
		t.Fatalf("404 custom = %d %q", rec.Code, rec.Body.String())
	}
}
