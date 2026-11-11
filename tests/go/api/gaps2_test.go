package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

// Gaps API parte 2: rutas sin cobertura HTTP (fs, detect, zones, hosts,
// server processes, cron delete/runs/log, alert-channels, proxy caps,
// logs/stream). Solo se testa lo que no cuelga (SSE se valida por método).
func TestGapsFSDetectZones(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// fs exige operator
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/fs", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("fs viewer: quiere 403, got %d", rec.Code)
	}
	// fs sin path: raíces
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/fs", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "entries") {
		t.Fatalf("fs raíces: %d %s", rec.Code, rec.Body.String())
	}
	// fs con path válido
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/fs?path="+t.TempDir(), opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("fs dir: %d %s", rec.Code, rec.Body.String())
	}
	// fs inexistente
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/fs?path="+filepath.Join(t.TempDir(), "noexiste-xyz"), opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("fs fantasma: quiere 404, got %d", rec.Code)
	}

	// detect exige operator y path
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/detect", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("detect viewer: quiere 403, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/detect", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("detect sin path: quiere 400, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/detect?path="+filepath.Join(t.TempDir(), "noexiste-xyz"), opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("detect fantasma: quiere 404, got %d", rec.Code)
	}
	// detect con package.json mínimo
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"start":"node s.js"}}`), 0o644)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/detect?path="+dir, opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"framework":"node"`) {
		t.Fatalf("detect node: %d %s", rec.Code, rec.Body.String())
	}

	// zones exige operator, devuelve array
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/zones", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("zones viewer: quiere 403, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/zones", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("zones: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGapsHostsServerProxy(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// hosts GET es público (lista del sistema)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/hosts", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hosts") {
		t.Fatalf("hosts lista: %d %s", rec.Code, rec.Body.String())
	}

	// server snapshot + history + processes (monitor real del NewHandler)
	for _, p := range []string{"/api/v1/server", "/api/v1/server/history?hours=1", "/api/v1/server/processes?limit=3"} {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, p, opTok))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", p, rec.Code, rec.Body.String())
		}
	}
	// método inválido
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/server", opTok))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("server POST: quiere 405, got %d", rec.Code)
	}

	// proxy capabilities: mapa estático de protocolos
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/proxy/capabilities", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "websocket") {
		t.Fatalf("proxy caps: %d %s", rec.Code, rec.Body.String())
	}

	// logs/stream no cuelga en test: método inválido 405, app inválida 400
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/stream?n=1", nil)
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("log stream POST: quiere 405, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/logs/stream?app=MALA!!!", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("log stream app inválida: quiere 400, got %d", rec.Code)
	}
}

func TestGapsCronRunsLogYDelete(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// runs global vacío
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/cron/runs", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("cron runs: %d %s", rec.Code, rec.Body.String())
	}
	// run log: id inválido 400, inexistente 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/cron/runs/abc/log", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cron log id inválido: quiere 400, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/cron/runs/999999/log", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cron log fantasma: quiere 404, got %d", rec.Code)
	}

	// alta + borrado de job
	req := httptest.NewRequest(http.MethodPut, "/api/v1/cron/w/tmp",
		strings.NewReader(`{"schedule":"* * * * *","command":"echo x","timeout":"1m"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cron put: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/cron/w/tmp", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("cron delete: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/cron/w/tmp", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cron delete fantasma: quiere 400, got %d", rec.Code)
	}
}

func TestGapsAlertChannelsCRUD(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	adminTok := loginAs(t, h, db, "admin", store.RoleAdmin)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// GET lista exige operator
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/alert-channels", adminTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("channels lista: %d %s", rec.Code, rec.Body.String())
	}

	// POST inválido (sin URL) 400
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alert-channels",
		strings.NewReader(`{"name":"c","url":"no-url"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("channel url mala: quiere 400, got %d", rec.Code)
	}

	// POST ok
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alert-channels",
		strings.NewReader(`{"name":"c","url":"https://example.com/hook"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("channel create: %d %s", rec.Code, rec.Body.String())
	}

	// PUT fantasma 400, id inválido 400
	req = httptest.NewRequest(http.MethodPut, "/api/v1/alert-channels/999999",
		strings.NewReader(`{"name":"c2","url":"https://example.com/hook2"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("channel put fantasma: quiere 400, got %d", rec.Code)
	}

	// DELETE fantasma 400
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/alert-channels/999999", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("channel delete fantasma: quiere 400, got %d", rec.Code)
	}
}
