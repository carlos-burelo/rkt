package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

func TestSetAppDomainsCanonico(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/w",
		strings.NewReader(`{"port":9,"domains":["a.test","b.test"]}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps", tok))
	var apps []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &apps); err != nil {
		t.Fatalf("json: %v", err)
	}
	found := false
	for _, a := range apps {
		if a["name"] == "w" {
			domains, _ := a["domains"].([]any)
			if len(domains) == 2 && domains[0] == "a.test" && domains[1] == "b.test" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("dominios no aplicados: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/v1/apps/w", strings.NewReader(`{"domains":[]}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/routes", tok))
	if strings.Contains(rec.Body.String(), "a.test") || strings.Contains(rec.Body.String(), "b.test") {
		t.Fatalf("rutas no retiradas: %s", rec.Body.String())
	}
}
