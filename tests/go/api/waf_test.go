package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/proxy"
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

	// operator ve reglas + bloqueos + baneos
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/waf", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deny_paths"`) ||
		!strings.Contains(rec.Body.String(), `"deny_methods"`) || !strings.Contains(rec.Body.String(), `"blocked"`) ||
		!strings.Contains(rec.Body.String(), `"banned"`) {
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

func TestWAFBansEndpoint(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// El proxy necesita una regla para denegar (testDeps no trae deny_paths).
	cp, err := cfgfile.MutateInfra(db, func(cp *config.Config) error {
		cp.DenyPaths = []string{"/.env"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	d.Live.Swap(cp)

	// sin ?ip
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/waf/bans", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unban sin ip: quiere 400, got %d", rec.Code)
	}
	// IP no baneada
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/waf/bans?ip=9.9.9.9", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unban fantasma: quiere 404, got %d", rec.Code)
	}
	// viewer no desbanea
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/waf/bans?ip=9.9.9.9", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unban viewer: quiere 403, got %d", rec.Code)
	}
	// método inválido
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/waf/bans?ip=9.9.9.9", opTok))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("bans GET: quiere 405, got %d", rec.Code)
	}

	// 20 golpes al proxy banean 192.0.2.1 (RemoteAddr de httptest)
	ph := proxy.NewHandler(d.Live, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < 20; i++ {
		r := httptest.NewRequest(http.MethodGet, "http://a.test/.env", nil)
		ph.ServeHTTP(httptest.NewRecorder(), r)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/waf", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "192.0.2.1") {
		t.Fatalf("baneado debe listarse: %d %s", rec.Code, rec.Body.String())
	}

	// desbanear lo saca de la lista (el historial de bloqueos se conserva)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/waf/bans?ip=192.0.2.1", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("unban: quiere 200, got %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/waf", opTok))
	var out struct {
		Banned  []map[string]any `json:"banned"`
		Blocked []map[string]any `json:"blocked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Banned) != 0 {
		t.Fatalf("tras unban no debe haber baneos: %+v", out.Banned)
	}
	if len(out.Blocked) == 0 {
		t.Fatal("el historial de bloqueos debe conservarse")
	}
}
