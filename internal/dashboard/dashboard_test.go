package dashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testDist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("x"), 0o644)
	return dir
}

// El HTML es público: si exigiera sesión, el wizard inicial sería
// inalcanzable (bug). Los datos siguen tras token en /api/v1.
func TestPublicoSinSesion(t *testing.T) {
	h := NewHandler(os.DirFS(testDist(t)))
	for _, p := range []string{"/dashboard/", "/dashboard/app.js", "/dashboard/ruta-spa/"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: quiere 200, got %d", p, rec.Code)
		}
	}
}

func TestSirveYFallback(t *testing.T) {
	h := NewHandler(os.DirFS(testDist(t)))

	for path, want := range map[string]string{
		"/dashboard/":          "<html>spa</html>",
		"/dashboard/app.js":    "x",
		"/dashboard/ruta-spa/": "<html>spa</html>", // fallback index
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Fatalf("%s: got %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

func Test503SinDist(t *testing.T) {
	h := NewHandler(os.DirFS(t.TempDir())) // vacío
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("quiere 503, got %d", rec.Code)
	}
}
