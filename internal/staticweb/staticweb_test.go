package staticweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
