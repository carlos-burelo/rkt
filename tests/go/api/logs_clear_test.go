package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"rkt/internal/store"
)

// POST /api/v1/logs/clear vacía los operativos (admin); viewer no puede.
func TestLogsClear(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)

	adm := loginAs(t, h, db, "adm", store.RoleAdmin)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)

	// viewer: 403 sin tocar nada.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/logs/clear", vis))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer debió dar 403, got %d", rec.Code)
	}

	// admin vacía: access.log tenía contenido en la fixture.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/logs/clear", adm))
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Files int `json:"files"`
		Bytes int `json:"bytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Files < 2 || out.Bytes <= 0 {
		t.Fatalf("debió vaciar archivos: %+v", out)
	}
	b, err := os.ReadFile(filepath.Join(d.LogsDir, "access.log"))
	if err != nil || len(b) != 0 {
		t.Fatalf("access.log no quedó vacío: %v %q", err, b)
	}

	// método erróneo.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/logs/clear", adm))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET debió dar 405, got %d", rec.Code)
	}

	// auditoría del POST.
	aud, err := db.ListAudit(50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range aud {
		if a.Action == "POST /api/v1/logs/clear" && a.Actor == "adm" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sin auditoría del clear: %+v", aud)
	}
}
