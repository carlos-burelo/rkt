package staticweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
