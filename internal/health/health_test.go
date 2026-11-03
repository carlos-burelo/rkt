package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"rkt-proxy/internal/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-health-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestOKAndDegraded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()
	host := upstream.Listener.Addr().String() // 127.0.0.1:puerto vivo

	path := writeTemp(t, fmt.Sprintf(`
routes:
  - hosts: ["ok.test"]
    target: "http://%s"
  - hosts: ["muerta.test"]
    target: "http://127.0.0.1:1"
`, host))
	defer os.Remove(path)
	cfg, err := config.Load(path)
	if err != nil {
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
	h := NewHandler(Deps{
		Live:    liveWithRoutes(t),
		Version: "x",
		Restart: func(name string) bool {
			if name != "web" {
				return false
			}
			restarted = name
			return true
		},
	})

	// POST ok
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/restart/web", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || restarted != "web" {
		t.Fatalf("POST web: code=%d restarted=%q", rec.Code, restarted)
	}

	// app inexistente -> 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/restart/otra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST otra: code=%d", rec.Code)
	}

	// GET -> 405, traversal -> 404
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/restart/web", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: code=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/restart/a/b", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal: code=%d", rec.Code)
	}
}

func liveWithRoutes(t *testing.T) *config.Live {
	t.Helper()
	path := writeTemp(t, "routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n")
	defer os.Remove(path)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return config.NewLive(cfg)
}

func TestNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(Deps{Live: liveWithRoutes(t), Version: "x"}).ServeHTTP(rec, httptest.NewRequest("GET", "/otra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestRetryEndpoint(t *testing.T) {
	h := NewHandler(Deps{
		Live:    liveWithRoutes(t),
		Version: "x",
		Retry: func(id int64, actor string) (string, error) {
			if id != 7 || actor != "local" {
				return "", fmt.Errorf("no")
			}
			return "rev7", nil
		},
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/retry/7", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "rev7") {
		t.Fatalf("POST retry: code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/retry/8", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("retry malo: code=%d", rec.Code)
	}
}
