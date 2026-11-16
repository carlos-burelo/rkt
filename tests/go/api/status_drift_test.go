package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rkt/internal/store"
)

// pending_restart viaja en /api/v1/status (deriva aplicada vs deseada).
func TestStatusPendingRestart(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.Pending = func() []string { return []string{"listen_http", "dashboard.listen"} }
	h := NewHandler(d)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+visTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var out struct {
		Pending []string `json:"pending_restart"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Pending) != 2 || out.Pending[0] != "listen_http" {
		t.Fatalf("pending_restart: %+v", out.Pending)
	}
}
