package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

func TestWAFEndpoints(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// viewer no ve bloqueos (expone IPs atacantes)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/waf", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("waf viewer: quiere 403, got %d", rec.Code)
	}

	// operator ve reglas + bloqueos
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/waf", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deny_paths"`) ||
		!strings.Contains(rec.Body.String(), `"deny_methods"`) || !strings.Contains(rec.Body.String(), `"blocked"`) {
		t.Fatalf("waf get: %d %s", rec.Code, rec.Body.String())
	}

	// PUT con patrón inválido no persiste
	req := httptest.NewRequest(http.MethodPut, "/api/v1/waf",
		strings.NewReader(`{"deny_paths":["/ok/*","/mal/["]}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("waf put malo: quiere 400, got %d", rec.Code)
	}

	// PUT válido reemplaza y normaliza (métodos a mayúsculas)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/waf",
		strings.NewReader(`{"deny_paths":["/ok/*"],"deny_methods":["trace","get"]}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"TRACE"`) {
		t.Fatalf("waf put: %d %s", rec.Code, rec.Body.String())
	}
	if got := d.Live.Get().DenyMethods; len(got) != 2 || got[0] != "TRACE" || got[1] != "GET" {
		t.Fatalf("live sin normalizar: %v", got)
	}

	// PUT parcial: solo métodos (paths se conservan)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/waf",
		strings.NewReader(`{"deny_methods":["TRACE"]}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("waf put parcial: %d %s", rec.Code, rec.Body.String())
	}
	if paths := d.Live.Get().DenyPaths; len(paths) != 1 || paths[0] != "/ok/*" {
		t.Fatalf("paths debieron conservarse: %v", paths)
	}

	// viewer no muta
	req = httptest.NewRequest(http.MethodPut, "/api/v1/waf",
		strings.NewReader(`{"deny_paths":[]}`))
	req.Header.Set("Authorization", "Bearer "+visTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("waf put viewer: quiere 403, got %d", rec.Code)
	}
}
