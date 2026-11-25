package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt/internal/store"
)

// Nuevos endpoints del studio sin Postgres: validación barata → 400 antes
// de conectar; escritura exige operator (viewer = 403).
func TestPgExtValidacionSinPG(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	op := loginAs(t, h, db, "op", store.RoleOperator)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)

	call := func(method, path, tok, body string) *httptest.ResponseRecorder {
		req := authed(method, path, tok)
		if body != "" {
			req.Body = io.NopCloser(strings.NewReader(body))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Viewer no escribe en los nuevos endpoints.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/pg/schema/index", `{"app":"w","table":"t","columns":["a"]}`},
		{http.MethodDelete, "/api/v1/pg/schema/index?app=w&index=i", ""},
		{http.MethodPost, "/api/v1/pg/schema/policy", `{"app":"w","table":"t","name":"p"}`},
		{http.MethodDelete, "/api/v1/pg/schema/policy?app=w&table=t&name=p", ""},
		{http.MethodPost, "/api/v1/pg/schema/rename", `{"app":"w","table":"a","new_table":"b"}`},
		{http.MethodPost, "/api/v1/pg/schema/column/rename", `{"app":"w","table":"t","column":"a","new_column":"b"}`},
		{http.MethodPost, "/api/v1/pg/schema/column/alter", `{"app":"w","table":"t","column":"a","not_null":true}`},
		{http.MethodPost, "/api/v1/pg/schema/truncate", `{"app":"w","table":"t"}`},
		{http.MethodPost, "/api/v1/pg/schema/duplicate", `{"app":"w","table":"a","new_table":"b"}`},
		{http.MethodDelete, "/api/v1/pg/backups?app=w&name=pg-w-20240101-000000.dump", ""},
		{http.MethodPost, "/api/v1/pg/rows/import", `{"app":"w","table":"t","columns":["a"],"rows":[[1]]}`},
		{http.MethodPost, "/api/v1/pg/query/batch", `{"app":"w","sql":"SELECT 1"}`},
		{http.MethodPost, "/api/v1/pg/query/explain", `{"app":"w","sql":"SELECT 1"}`},
	} {
		if rec := call(tc.method, tc.path, vis, tc.body); rec.Code != http.StatusForbidden {
			t.Fatalf("viewer %s %s: quiere 403, got %d (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	// Cuerpos inválidos: 400 antes de conectar.
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/pg/schema/index", `{"app":"w","table":"MAL","columns":["a"]}`},
		{http.MethodPost, "/api/v1/pg/schema/index", `{"app":"w","table":"t","columns":[]}`},
		{http.MethodPost, "/api/v1/pg/schema/index", `{"app":"w","table":"t","columns":["a"],"method":"otro"}`},
		{http.MethodPost, "/api/v1/pg/schema/policy", `{"app":"w","table":"t","name":"p","cmd":"otro"}`},
		{http.MethodPost, "/api/v1/pg/schema/policy", `{"app":"w","table":"t","name":"p","using":"a;b"}`},
		{http.MethodPost, "/api/v1/pg/schema/rename", `{"app":"w","table":"a","new_table":"a"}`},
		{http.MethodPost, "/api/v1/pg/schema/rename", `{"app":"w","table":"MAL","new_table":"b"}`},
		{http.MethodPost, "/api/v1/pg/schema/column/rename", `{"app":"w","table":"t","column":"a","new_column":"a"}`},
		{http.MethodPost, "/api/v1/pg/schema/column/alter", `{"app":"w","table":"t","column":"a"}`},
		{http.MethodPost, "/api/v1/pg/schema/column/alter", `{"app":"w","table":"t","column":"a","type":"otro"}`},
		{http.MethodPost, "/api/v1/pg/schema/duplicate", `{"app":"w","table":"a","new_table":"a"}`},
		{http.MethodPost, "/api/v1/pg/rows/import", `{"app":"w","table":"t","columns":[],"rows":[]}`},
		{http.MethodPost, "/api/v1/pg/rows/import", `{"app":"w","table":"t","columns":["a"],"rows":[[1,2]]}`},
		{http.MethodPost, "/api/v1/pg/query/batch", `{"app":"w","sql":""}`},
	} {
		if rec := call(tc.method, tc.path, op, tc.body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s %s: quiere 400, got %d (%s)", tc.method, tc.path, tc.body, rec.Code, rec.Body.String())
		}
	}

	// Lote de 21 sentencias: 400 sin tocar Postgres.
	many := strings.Repeat("SELECT 1; ", 21)
	if rec := call(http.MethodPost, "/api/v1/pg/query/batch", op, `{"app":"w","sql":`+quoteJSON(many)+`}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("batch 21: quiere 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Borrado de backup con traversal: 400.
	if rec := call(http.MethodDelete, "/api/v1/pg/backups?app=w&name=../x.dump", op, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("backup traversal: quiere 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Export con filtro malo: 400 sin conectar.
	if rec := call(http.MethodGet, "/api/v1/pg/rows/export?app=w&table=t&filter=malformado", op, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("export filtro: quiere 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Sin base: 400 honesto (no 500) en los que resuelven esquema.
	for _, p := range []string{
		"/api/v1/pg/schema/indexes?app=w",
		"/api/v1/pg/schema/constraints?app=w",
		"/api/v1/pg/rows/export?app=w&table=t",
		"/api/v1/pg/backups/download?app=w&name=pg-w-20240101-000000.dump",
	} {
		if rec := call(http.MethodGet, p, op, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s sin base: quiere 400, got %d (%s)", p, rec.Code, rec.Body.String())
		}
	}
}

// Objetos del esquema sin Postgres: validación barata → 400 antes de
// conectar; escritura exige operator (viewer = 403).
func TestPgObjValidacionSinPG(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	op := loginAs(t, h, db, "op", store.RoleOperator)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)

	call := func(method, path, tok, body string) *httptest.ResponseRecorder {
		req := authed(method, path, tok)
		if body != "" {
			req.Body = io.NopCloser(strings.NewReader(body))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/pg/schema/view", `{"app":"w","name":"v","sql":"SELECT 1"}`},
		{http.MethodDelete, "/api/v1/pg/schema/view?app=w&name=v", ""},
		{http.MethodPost, "/api/v1/pg/schema/matview/refresh", `{"app":"w","name":"m"}`},
		{http.MethodDelete, "/api/v1/pg/schema/function?app=w&func=f()", ""},
		{http.MethodPost, "/api/v1/pg/schema/procedure", `{"app":"w","procedure":"p(1)"}`},
		{http.MethodDelete, "/api/v1/pg/schema/procedure?app=w&procedure=p()", ""},
		{http.MethodPost, "/api/v1/pg/schema/trigger", `{"app":"w","table":"t","name":"tr","timing":"before","events":["insert"],"function":"f()"}`},
		{http.MethodDelete, "/api/v1/pg/schema/trigger?app=w&table=t&name=tr", ""},
		{http.MethodPost, "/api/v1/pg/schema/trigger/toggle", `{"app":"w","table":"t","name":"tr","enable":false}`},
		{http.MethodPost, "/api/v1/pg/schema/sequence/restart", `{"app":"w","name":"q"}`},
		{http.MethodPost, "/api/v1/pg/schema/sequence/setval", `{"app":"w","name":"q","value":1}`},
	} {
		if rec := call(tc.method, tc.path, vis, tc.body); rec.Code != http.StatusForbidden {
			t.Fatalf("viewer %s %s: quiere 403, got %d (%s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/pg/schema/view", `{"app":"w","name":"MAL","sql":"SELECT 1"}`},
		{http.MethodPost, "/api/v1/pg/schema/view", `{"app":"w","name":"v","sql":"DELETE FROM t"}`},
		{http.MethodPost, "/api/v1/pg/schema/view", `{"app":"w","name":"v","sql":"SELECT 1; SELECT 2"}`},
		{http.MethodPost, "/api/v1/pg/schema/trigger", `{"app":"w","table":"t","name":"tr","timing":"otro","events":["insert"],"function":"f()"}`},
		{http.MethodPost, "/api/v1/pg/schema/trigger", `{"app":"w","table":"t","name":"tr","timing":"before","events":[],"function":"f()"}`},
		{http.MethodPost, "/api/v1/pg/schema/procedure", `{"app":"w","procedure":"p(1); SELECT"}`},
	} {
		if rec := call(tc.method, tc.path, op, tc.body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s %s: quiere 400, got %d (%s)", tc.method, tc.path, tc.body, rec.Code, rec.Body.String())
		}
	}

	for _, p := range []string{
		"/api/v1/pg/schema/views?app=w",
		"/api/v1/pg/schema/views?app=w&name=v",
		"/api/v1/pg/schema/functions?app=w",
		"/api/v1/pg/schema/procedures?app=w",
		"/api/v1/pg/schema/triggers?app=w",
		"/api/v1/pg/schema/sequences?app=w",
	} {
		if rec := call(http.MethodGet, p, op, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s sin base: quiere 400, got %d (%s)", p, rec.Code, rec.Body.String())
		}
	}
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
