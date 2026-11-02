package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/store"
)

func testDeps(t *testing.T, token string) (Deps, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		if err := db.SetSetting("dashboard_token", token); err != nil {
			t.Fatal(err)
		}
	}
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(filepath.Join(logsDir, "apps"), 0o755)
	os.WriteFile(filepath.Join(logsDir, "rkt.log"), []byte("l1\nl2\nl3\n"), 0o644)
	cfg := &config.Config{Routes: []config.Route{
		{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"},
	}}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return Deps{
		DB: db, Live: config.NewLive(cfg), Version: "test",
		LogsDir: logsDir,
		Apps:    func() []health.AppStatus { return []health.AppStatus{{Name: "w", Running: true}} },
	}, func() { db.Close() }
}

func authed(req *http.Request) {
	req.Header.Set("Authorization", "Bearer tok-secreto")
}

func TestSinTokenFailClosed(t *testing.T) {
	d, done := testDeps(t, "")
	defer done()
	h := NewHandler(d)
	for _, p := range []string{"/api/v1/status", "/api/v1/deployments", "/api/v1/events", "/api/v1/logs"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		authed(req)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: quiere 401 sin token configurado, got %d", p, rec.Code)
		}
	}
}

func TestTokenInvalido(t *testing.T) {
	d, done := testDeps(t, "tok-secreto")
	defer done()
	h := NewHandler(d)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer otro")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("quiere 401, got %d", rec.Code)
	}
}

func TestStatusYLogs(t *testing.T) {
	d, done := testDeps(t, "tok-secreto")
	defer done()
	h := NewHandler(d)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	authed(req)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var st struct {
		Version string             `json:"version"`
		Routes  int                `json:"routes"`
		Apps    []health.AppStatus `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Version != "test" || st.Routes != 1 || len(st.Apps) != 1 || st.Apps[0].Name != "w" {
		t.Fatalf("status mal: %+v", st)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/logs?n=2", nil)
	authed(req)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs: %d", rec.Code)
	}
	var lg struct {
		Log string `json:"log"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lg); err != nil {
		t.Fatal(err)
	}
	if lg.Log != "l2\nl3" {
		t.Fatalf("tail mal: %q", lg.Log)
	}

	// path traversal bloqueado por validAppName
	req = httptest.NewRequest(http.MethodGet, "/api/v1/logs?app=../x", nil)
	authed(req)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal: quiere 400, got %d", rec.Code)
	}
}

func TestDeploymentsEventsVacios(t *testing.T) {
	d, done := testDeps(t, "tok-secreto")
	defer done()
	h := NewHandler(d)
	for _, p := range []string{"/api/v1/deployments", "/api/v1/events"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		authed(req)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", p, rec.Code)
		}
	}
}
