package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"rkt/internal/store"
)

// handlePg sin pg_mode local: informa desactivado sin exigir nada más.
func TestPgDesactivado(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/pg", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET pg: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Enabled {
		t.Fatalf("sin pg_mode debe salir desactivado: %s", rec.Body.String())
	}

	// POST también responde desactivado sin pedir app válida.
	req := authed(http.MethodPost, "/api/v1/pg", tok)
	req.Body = io.NopCloser(strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST sin pg_mode: %d %s", rec.Code, rec.Body.String())
	}

	// Viewer no muta (aunque esté desactivado, el gate responde antes).
	vis := loginAs(t, h, db, "vis", store.RoleViewer)
	if err := db.SetSetting("pg_mode", "local"); err != nil {
		t.Fatal(err)
	}
	req = authed(http.MethodDelete, "/api/v1/pg/x", vis)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer delete: quiere 403, got %d", rec.Code)
	}

	// Con pg_mode local (bins falsos en temp, sin servidor): la validación
	// va antes que la red.
	binDir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	for _, n := range []string{"pg_ctl", "psql", "pg_dump", "pg_restore", "initdb"} {
		if err := os.WriteFile(filepath.Join(binDir, n+ext), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetSetting("pg_bin", filepath.Join(binDir, "pg_ctl"+ext)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{}`, http.StatusBadRequest},
		{`{"app":"MAL_NOMBRE"}`, http.StatusBadRequest},
		{`{"app":"zz"}`, http.StatusBadRequest},
		{`{"app":"w","mode":"x"}`, http.StatusInternalServerError},
	} {
		req = authed(http.MethodPost, "/api/v1/pg", tok)
		req.Body = io.NopCloser(strings.NewReader(tc.body))
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("body %s: quiere %d, got %d (%s)", tc.body, tc.code, rec.Code, rec.Body.String())
		}
	}
}

// Flujo real contra Postgres local (puerta: RKT_PG_TEST_DSN con superuser
// y RKT_PG_TEST_PASS). En CI se salta.
func TestPgApiLive(t *testing.T) {
	if os.Getenv("RKT_PG_TEST_DSN") == "" {
		t.Skip("sin RKT_PG_TEST_DSN")
	}
	pgBin := `C:\Program Files\PostgreSQL\18\bin\pg_ctl.exe`
	if _, err := os.Stat(pgBin); err != nil {
		t.Skipf("sin postgres local: %v", err)
	}
	d, db, done := testDeps(t)
	defer done()
	for k, v := range map[string]string{
		"pg_mode": "local", "pg_bin": pgBin,
		"pg_pass": os.Getenv("RKT_PG_TEST_PASS"),
	} {
		if err := db.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	// Provisiona la app de prueba de la fixture.
	body := strings.NewReader(`{"app":"w","mode":"dedicated"}`)
	req := authed(http.MethodPost, "/api/v1/pg", tok)
	req.Body = io.NopCloser(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("provision: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Schema string `json:"schema"`
		Role   string `json:"role"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Schema != "w" || created.Role != "w" || !strings.Contains(created.URL, "postgres://w@") {
		t.Fatalf("creada mal: %+v", created)
	}

	// Aparece en el listado con URL.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/pg", tok))
	var list struct {
		Enabled bool `json:"enabled"`
		Apps    []struct {
			App string `json:"app"`
			URL string `json:"url"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || !list.Enabled || len(list.Apps) != 1 {
		t.Fatalf("lista: %d %s", rec.Code, rec.Body.String())
	}

	// Consola: crea tabla + fila en su esquema (search_path del rol).
	post := func(path, body string) *httptest.ResponseRecorder {
		req := authed(http.MethodPost, path, tok)
		req.Body = io.NopCloser(strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for _, q := range []string{
		`{"app":"w","sql":"CREATE TABLE t1 (id int primary key, v text)"}`,
		`{"app":"w","sql":"INSERT INTO t1 VALUES (1, 'hola')"}`,
	} {
		if rec := post("/api/v1/pg/query", q); rec.Code != http.StatusOK {
			t.Fatalf("query %s: %d %s", q, rec.Code, rec.Body.String())
		}
	}
	// Una sola sentencia por ejecución.
	if rec := post("/api/v1/pg/query", `{"app":"w","sql":"SELECT 1; SELECT 2"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("multi-sentencia debe fallar: %d %s", rec.Code, rec.Body.String())
	}

	// Tablas contiene t1.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/pg/tables?app=w", tok))
	var tbls struct {
		Tables []string `json:"tables"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tbls); err != nil || len(tbls.Tables) != 1 || tbls.Tables[0] != "t1" {
		t.Fatalf("tablas: %d %s", rec.Code, rec.Body.String())
	}

	// Filas: SELECT via consola y paginado.
	if rec := post("/api/v1/pg/query", `{"app":"w","sql":"SELECT v FROM t1"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hola") {
		t.Fatalf("select: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/pg/rows?app=w&table=t1&n=10&page=1", tok))
	var rows struct {
		Columns []string `json:"columns"`
		Types   []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"types"`
		Rows  [][]any `json:"rows"`
		Total int64   `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || rows.Total != 1 || len(rows.Rows) != 1 {
		t.Fatalf("rows: %d %s", rec.Code, rec.Body.String())
	}
	if len(rows.Types) != 2 || rows.Types[0].Type == "" || rows.Types[1].Type == "" {
		t.Fatalf("rows sin tipos: %+v", rows.Types)
	}

	// Backup genera dump.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/pg/w/backup", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("backup: %d %s", rec.Code, rec.Body.String())
	}
	var bk struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bk); err != nil || bk.Path == "" {
		t.Fatalf("backup path: %s %v", rec.Body.String(), err)
	}
	if st, err := os.Stat(bk.Path); err != nil || st.Size() == 0 {
		t.Fatalf("dump vacío: %v", bk.Path)
	}
	defer os.RemoveAll(filepath.Dir(bk.Path))

	// Restore: tras vaciar la tabla, el dump la devuelve.
	if rec := post("/api/v1/pg/query", `{"app":"w","sql":"DELETE FROM t1"}`); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post("/api/v1/pg/restore", `{"app":"w","file":`+strconv.Quote(bk.Path)+`}`); rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/pg/rows?app=w&table=t1&n=10&page=1", tok))
	rows = struct {
		Columns []string `json:"columns"`
		Types   []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"types"`
		Rows  [][]any `json:"rows"`
		Total int64   `json:"total"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || rows.Total != 1 {
		t.Fatalf("rows tras restore: %d %s", rec.Code, rec.Body.String())
	}
	// SELECT con tipos (cabeceras del editor).
	if rec := post("/api/v1/pg/query", `{"app":"w","sql":"SELECT id, v FROM t1"}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"types"`) {
		t.Fatalf("select sin types: %d %s", rec.Code, rec.Body.String())
	}

	// Drop limpia registro y esquema.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/pg/w", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("drop: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/pg", tok))
	list.Apps = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Apps) != 0 {
		t.Fatalf("vacía tras drop: %s", rec.Body.String())
	}
}
