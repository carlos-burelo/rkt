package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

func TestDomainSuggestSegunModo(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	if err := db.SetSetting(store.KeyBaseDomain, "ejemplo.com"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/suggest?name=tienda", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("local: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "tienda.localhost") || strings.Contains(body, `"requires_domain":true`) {
		t.Fatalf("local = %s", body)
	}

	// Público sin base: no inventa localhost y pide dominio con acceso.
	if err := db.SetSetting(store.KeyPublicIP, "203.0.113.7"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(store.KeyBaseDomain, ""); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/suggest?name=tienda", tok))
	body = rec.Body.String()
	if strings.Contains(body, "localhost") || !strings.Contains(body, `"requires_domain":true`) ||
		!strings.Contains(body, `"public":true`) {
		t.Fatalf("público = %s", body)
	}
}
