package api

// Rutas por path vía API: alta, listado, retarget y borrado por selector;
// el legacy por host queda intacto.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

func postJSON(t *testing.T, h http.Handler, tok, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestRutasPathAPI(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// Alta por path (vale sin hosts: cualquier host/IP).
	if code, body := postJSON(t, h, opTok, "/api/v1/routes", `{"hosts":["a.test"],"path":"/panel","target":"http://127.0.0.1:9"}`); code != http.StatusCreated {
		t.Fatalf("alta path: %d %s", code, body)
	}
	if code, body := postJSON(t, h, opTok, "/api/v1/routes", `{"path":"/pub","target":"http://127.0.0.1:10"}`); code != http.StatusCreated {
		t.Fatalf("alta path sin host: %d %s", code, body)
	}
	// Duplicado exacto (host+path) se rechaza.
	if code, _ := postJSON(t, h, opTok, "/api/v1/routes", `{"hosts":["a.test"],"path":"/panel","target":"http://127.0.0.1:11"}`); code != http.StatusBadRequest {
		t.Fatalf("duplicado: %d", code)
	}

	// El listado expone path.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/routes", opTok))
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var seenPath, seenBare bool
	for _, r := range got {
		if r["path"] == "/panel" {
			seenPath = true
		}
		if r["path"] == "/pub" {
			seenBare = true
		}
	}
	if !seenPath || !seenBare {
		t.Fatalf("listado sin path: %s", rec.Body.String())
	}

	// Retarget por selector completo.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/routes", strings.NewReader(`{"host":"a.test","path":"/panel","target":"http://127.0.0.1:99"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "1") {
		t.Fatalf("retarget: %d %s", rec.Code, rec.Body.String())
	}

	// Borrado por path (con y sin host).
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/routes?host=a.test&path=/panel", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("borrado path: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/routes?path=/pub", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("borrado path suelto: %d %s", rec.Code, rec.Body.String())
	}

	// Legacy por host intacto: borrar lo que no existe da 404.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/routes?host=fantasma.test", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("legacy 404: %d", rec.Code)
	}
	// Sin selector: 400, no borra todo.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/routes", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin selector: %d", rec.Code)
	}
}
