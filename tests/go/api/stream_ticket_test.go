package api

// Tickets SSE: poseedor de rkt_* sin cookie abre streams con ?ticket=
// (un uso, 60s). Sin ticket válido el stream sigue exigiendo sesión.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rkt/internal/store"
)

func mintTicket(t *testing.T, h http.Handler, tok string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stream/ticket", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil || out.Ticket == "" {
		t.Fatalf("mint sin ticket: %s", rec.Body.String())
	}
	return out.Ticket
}

func TestStreamTicket(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)
	srv := httptest.NewServer(h)
	defer srv.Close()

	// El ticket abre el stream de logs y lee el init.
	tick := mintTicket(t, h, visTok)
	evs := fleetEvents(t, srv.URL+"/api/v1/logs/stream?ticket="+tick, 1)
	if len(evs) != 1 {
		t.Fatalf("sin init: %+v", evs)
	}

	// Un solo uso: reutilizar da 401.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/logs/stream?ticket="+tick, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reuse: %d, quiere 401", resp.StatusCode)
	}

	// Ticket ajeno también da 401.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/logs/stream?ticket=muerto", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("malo: %d, quiere 401", resp.StatusCode)
	}

	// Sin nada sigue 401.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/v1/logs/stream", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sin ticket: %d, quiere 401", resp.StatusCode)
	}
}
