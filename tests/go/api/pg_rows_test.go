package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt/internal/store"
)

// Visor Supabase sin Postgres: la validación barata (filtros, orden,
// identificadores, cuerpos) responde 400 antes de conectar; la escritura
// exige operator (viewer = 403).
func TestPgRowsValidacionSinPG(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	op := loginAs(t, h, db, "op", store.RoleOperator)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)

	call := func(method, path, tok, body string) *httptest.ResponseRecorder {
		var req *http.Request
		if body == "" {
			req = authed(method, path, tok)
		} else {
			req = authed(method, path, tok)
			req.Body = io.NopCloser(strings.NewReader(body))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Viewer no escribe filas.
	if rec := call(http.MethodPost, "/api/v1/pg/rows", vis, `{"app":"w","table":"t","data":{"a":1}}`); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer POST rows: quiere 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPatch, "/api/v1/pg/rows", vis, `{"app":"w","table":"t","pk":{"id":1},"patch":{"a":1}}`); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer PATCH rows: quiere 403, got %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/api/v1/pg/rows", vis, `{"app":"w","table":"t","pk":{"id":1}}`); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer DELETE rows: quiere 403, got %d", rec.Code)
	}

	// Filtros y orden inválidos: 400 sin tocar Postgres.
	for _, p := range []string{
		"/api/v1/pg/rows?app=w&table=t&filter=malformado",
		"/api/v1/pg/rows?app=w&table=t&filter=a:otro:1",
		"/api/v1/pg/rows?app=w&table=MAL&filter=a:eq:1",
		"/api/v1/pg/rows?app=w&table=t&order=id&dir=sideways",
	} {
		if rec := call(http.MethodGet, p, op, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s: quiere 400, got %d (%s)", p, rec.Code, rec.Body.String())
		}
	}

	// Cuerpos inválidos: 400 antes de conectar.
	for _, tc := range []struct {
		method, body string
	}{
		{http.MethodPost, `{"app":"w","table":"t","data":{}}`},
		{http.MethodPost, `{"app":"w","table":"t","data":{"MAL":1}}`},
		{http.MethodPatch, `{"app":"w","table":"t","patch":{"a":1}}`},
		{http.MethodPatch, `{"app":"w","table":"t","pk":{"id":1},"patch":{}}`},
		{http.MethodDelete, `{"app":"w","table":"t"}`},
		{http.MethodDelete, `{"app":"w","table":"t","pk":{"id":1},"filters":["a:eq:1"]}`},
	} {
		if rec := call(tc.method, "/api/v1/pg/rows", op, tc.body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: quiere 400, got %d (%s)", tc.method, tc.body, rec.Code, rec.Body.String())
		}
	}

	// DDL inválido: 400 antes de conectar.
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/v1/pg/schema", `{"app":"w","table":"MAL","columns":[{"name":"id","type":"text"}]}`},
		{http.MethodPost, "/api/v1/pg/schema", `{"app":"w","table":"t","columns":[]}`},
		{http.MethodPost, "/api/v1/pg/schema", `{"app":"w","table":"t","columns":[{"name":"a","type":"otro"}]}`},
		{http.MethodPost, "/api/v1/pg/schema/column", `{"app":"w","table":"t","column":{"name":"a","type":"otro"}}`},
	} {
		if rec := call(tc.method, tc.path, op, tc.body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s: quiere 400, got %d (%s)", tc.method, tc.body, rec.Code, rec.Body.String())
		}
	}

	// Sin base aprovisionada: 400 honesto (no 500).
	for _, p := range []string{
		"/api/v1/pg/tables?app=w",
		"/api/v1/pg/rows?app=w&table=t",
		"/api/v1/pg/schema?app=w&table=t",
	} {
		if rec := call(http.MethodGet, p, op, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s sin base: quiere 400, got %d (%s)", p, rec.Code, rec.Body.String())
		}
	}
}
