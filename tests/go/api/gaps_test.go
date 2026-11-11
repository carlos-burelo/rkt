package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

// Gaps API parte 1: auth/me, tokens, events, deployments lista/detalle,
// app diff/build, env global import. Sin estos el "funcionamiento total"
// no estaba corroborado a nivel HTTP.
func TestGapsAuthMeYTokens(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	adminTok := loginAs(t, h, db, "admin", store.RoleAdmin)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// /auth/me devuelve usuario y rol de la sesión
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/auth/me", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"user":"op"`) {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}

	// viewer no puede listar ni crear tokens
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/tokens", opTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tokens operator: quiere 403, got %d", rec.Code)
	}

	// admin lista vacío
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/tokens", adminTok))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) == "null" {
		t.Fatalf("tokens lista: %d %s", rec.Code, rec.Body.String())
	}

	// admin crea token
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens",
		strings.NewReader(`{"name":"ci","role":"operator"}`))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "rkt_") {
		t.Fatalf("token create: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !strings.HasPrefix(created.Token, "rkt_") {
		t.Fatalf("token sin prefijo rkt_: %q", created.Token)
	}

	// lista ya trae 1, sin exponer hash
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/tokens", adminTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ci"`) ||
		strings.Contains(strings.ToLower(rec.Body.String()), "hash") {
		t.Fatalf("tokens lista con 1: %d %s", rec.Code, rec.Body.String())
	}
	var list []store.APIToken
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("tokens = %+v", list)
	}

	// el token de API autentica (TokenUser) aunque no sea sesión
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/auth/me", created.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("me con api token: %d %s", rec.Code, rec.Body.String())
	}

	// revocar
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/tokens/999999", adminTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoke fantasma: quiere 404, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/tokens/abc", adminTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke id inválido: quiere 400, got %d", rec.Code)
	}
}

func TestGapsEventsYDeployments(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// events vacío + filtro por kind
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/events?n=10", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("events: %d %s", rec.Code, rec.Body.String())
	}
	if err := db.InsertEvent("died", "w", "boom"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/events?kind=died&app=w", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("events filtro: %d %s", rec.Code, rec.Body.String())
	}

	// deployments lista vacía, luego con 1 fila
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/deployments?n=10", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("deployments lista: %d %s", rec.Code, rec.Body.String())
	}
	id, err := db.InsertDeployment("w", "abc123", "", "op")
	if err != nil {
		t.Fatal(err)
	}
	_ = id
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/deployments?app=w", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("deployments con 1: %d %s", rec.Code, rec.Body.String())
	}

	// detalle por id
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/deployments/999999", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("detalle fantasma: quiere 404, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/deployments/abc", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("detalle id inválido: quiere 400, got %d", rec.Code)
	}
}

func TestGapsAppDiffBuildEnvImport(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// diff exige operator y from/to
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w/diff?from=a&to=b", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("diff viewer: quiere 403, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w/diff", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("diff sin from/to: quiere 400, got %d", rec.Code)
	}
	// app inexistente
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/nadie/diff?from=a&to=b", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("diff fantasma: quiere 404, got %d", rec.Code)
	}

	// build: operator ok (stub devuelve 1), viewer 403
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/apps/w/build", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("build viewer: quiere 403, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/apps/w/build", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"deployment":1`) {
		t.Fatalf("build: %d %s", rec.Code, rec.Body.String())
	}
	// build deshabilitado (sin stub)
	d2, db2, done2 := testDeps(t)
	defer done2()
	d2.Build = nil
	h2 := NewHandler(d2)
	op2 := loginAs(t, h2, db2, "op", store.RoleOperator)
	rec = httptest.NewRecorder()
	h2.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/apps/w/build", op2))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("build sin backend: quiere 503, got %d", rec.Code)
	}

	// env global import: operator ok, viewer 403, JSON inválido 400
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/env/global/import", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("env import viewer: quiere 403, got %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/env/global/import",
		strings.NewReader(`{"raw":"G1=1\nG2=${G1}-2\n"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "G1") {
		t.Fatalf("env import: %d %s", rec.Code, rec.Body.String())
	}
	// valor expandido persiste
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/env/global/G2", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "1-2") {
		t.Fatalf("env import expandido: %d %s", rec.Code, rec.Body.String())
	}
}
