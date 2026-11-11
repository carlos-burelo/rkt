package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/servermetrics"
	"rkt-proxy/internal/store"
)

func TestServerSnapshot(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.Server = servermetrics.NewMonitor()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/server", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"memory"`) {
		t.Fatalf("server: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/server/history?hours=1", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("history: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/server", tok))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("método: %d", rec.Code)
	}
}

func TestServerDoctor(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/server/doctor", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("doctor: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"name":"git"`, `"state":`, `"detail":`} {
		if !strings.Contains(body, want) {
			t.Fatalf("doctor sin %s: %s", want, body)
		}
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/server/doctor", tok))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("método: %d", rec.Code)
	}
}
