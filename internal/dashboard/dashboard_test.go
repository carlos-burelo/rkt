package dashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rkt-proxy/internal/store"
)

func testDB(t *testing.T) (*store.DB, *http.Cookie) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.CreateUser("adm", "password-larga-1", store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	tok, err := db.NewSession("adm", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return db, &http.Cookie{Name: "rkt_token", Value: tok}
}

func testDist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("x"), 0o644)
	return dir
}

func Test401SinSesion(t *testing.T) {
	db, _ := testDB(t)
	h := NewHandler(db, os.DirFS(testDist(t)))
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("quiere 401, got %d", rec.Code)
	}
}

func TestSirveYFallback(t *testing.T) {
	db, cookie := testDB(t)
	h := NewHandler(db, os.DirFS(testDist(t)))

	for path, want := range map[string]string{
		"/dashboard/":          "<html>spa</html>",
		"/dashboard/app.js":    "x",
		"/dashboard/ruta-spa/": "<html>spa</html>", // fallback index
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Fatalf("%s: got %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

func Test503SinDist(t *testing.T) {
	db, cookie := testDB(t)
	h := NewHandler(db, os.DirFS(t.TempDir())) // vacío
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("quiere 503, got %d", rec.Code)
	}
}
