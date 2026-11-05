package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/store"
)

func TestUserPasswordSessionsAudit(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	adminTok := loginAs(t, h, db, "admin", store.RoleAdmin)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	// sesión propia listada
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/users/me/sessions", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sessions"`) {
		t.Fatalf("sessions: %d %s", rec.Code, rec.Body.String())
	}
	var sess struct {
		Sessions []store.Session `json:"sessions"`
		Current  string          `json:"current"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	if len(sess.Sessions) == 0 || sess.Current == "" {
		t.Fatalf("sesiones = %+v", sess)
	}
	// segunda sesión y revocar la otra (no la actual)
	otherTok := login(t, h, "op", "password-larga-1")
	_ = otherTok
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/users/me/sessions", opTok))
	_ = json.Unmarshal(rec.Body.Bytes(), &sess)
	victim := ""
	for _, s := range sess.Sessions {
		if s.ID != sess.Current {
			victim = s.ID
		}
	}
	if victim == "" {
		t.Fatalf("sin segunda sesión: %+v", sess)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/users/me/sessions/"+victim, opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	// cambio de contraseña propio (nueva sesión)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/password",
		strings.NewReader(`{"current":"password-larga-1","next":"password-nueva-99"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("password: %d %s", rec.Code, rec.Body.String())
	}
	// admin deshabilita a op
	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/op", strings.NewReader(`{"disabled":true}`))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	// auditoría visible
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/audit", adminTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "user.disabled") {
		t.Fatalf("audit: %d %s", rec.Code, rec.Body.String())
	}
}
