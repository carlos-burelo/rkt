package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/config"
)

func liveWithRoutes(t *testing.T) *config.Live {
	t.Helper()
	cfg := config.Default()
	cfg.Routes = []config.Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"}}
	if err := cfg.Finalize(); err != nil {
		t.Fatal(err)
	}
	return config.NewLive(cfg)
}

func TestOKAndDegraded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()
	host := upstream.Listener.Addr().String() // 127.0.0.1:puerto vivo

	cfg := config.Default()
	cfg.Routes = []config.Route{
		{Hosts: []string{"ok.test"}, Target: "http://" + host},
		{Hosts: []string{"muerta.test"}, Target: "http://127.0.0.1:1"},
	}
	if err := cfg.Finalize(); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{Live: config.NewLive(cfg), Version: "test"})

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	var st struct {
		Status   string `json:"status"`
		Version  string `json:"version"`
		Backends []struct {
			Target string `json:"target"`
			OK     bool   `json:"ok"`
		} `json:"backends"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Status != "degraded" || st.Version != "test" || len(st.Backends) != 2 {
		t.Fatalf("inesperado: %+v", st)
	}
	if !st.Backends[0].OK || st.Backends[1].OK {
		t.Fatalf("checks mal: %+v", st.Backends)
	}
}

func TestRestartEndpoint(t *testing.T) {
	var restarted string
	const token = "token-de-control-de-prueba"
	h := NewHandler(Deps{
		Live:    liveWithRoutes(t),
		Version: "x",
		Token:   token,
		Restart: func(name string) bool {
			if name != "web" {
				return false
			}
			restarted = name
			return true
		},
	})
	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("X-Rkt-Control", token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// sin token -> 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/restart/web", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: code=%d", rec.Code)
	}

	// POST ok
	rec = post("/restart/web")
	if rec.Code != http.StatusOK || restarted != "web" {
		t.Fatalf("POST web: code=%d restarted=%q", rec.Code, restarted)
	}

	// app inexistente -> 404
	rec = post("/restart/otra")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST otra: code=%d", rec.Code)
	}

	// GET sin token -> 401; traversal con token -> 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/restart/web", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET sin token: code=%d", rec.Code)
	}
	rec = post("/restart/a/b")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal: code=%d", rec.Code)
	}
}

func TestNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(Deps{Live: liveWithRoutes(t), Version: "x"}).ServeHTTP(rec, httptest.NewRequest("GET", "/otra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestRetryEndpoint(t *testing.T) {
	const token = "token-de-control-de-prueba"
	h := NewHandler(Deps{
		Live:    liveWithRoutes(t),
		Version: "x",
		Token:   token,
		Retry: func(id int64, actor string) (string, error) {
			if id != 7 || actor != "local" {
				return "", fmt.Errorf("no")
			}
			return "rev7", nil
		},
	})
	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := post("/retry/7")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "rev7") {
		t.Fatalf("POST retry: code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = post("/retry/8")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("retry malo: code=%d", rec.Code)
	}
}

// TestHealthzAppsEstado: el healthz refleja el estado real de la flota
// (caída = degraded) y expone pid/reinicios.
func TestHealthzAppsEstado(t *testing.T) {
	h := NewHandler(Deps{
		Live:    liveWithRoutes(t),
		Version: "x",
		Apps: func() []AppStatus {
			return []AppStatus{
				{Name: "web", Running: true, Pid: 123, HTTPOK: true},
				{Name: "caida", Running: false, Failed: true, Restarts: 3},
			}
		},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var st struct {
		Status string      `json:"status"`
		Apps   []AppStatus `json:"apps"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Status != "degraded" {
		t.Fatalf("estado = %q, quiere degraded", st.Status)
	}
	if len(st.Apps) != 2 || st.Apps[0].Pid != 123 || st.Apps[1].Restarts != 3 {
		t.Fatalf("apps = %+v", st.Apps)
	}
}
