package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	h := NewHandler(cfg, "test")

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

func TestNotFound(t *testing.T) {
	path := writeTemp(t, "routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n")
	defer os.Remove(path)
	cfg, _ := config.Load(path)
	rec := httptest.NewRecorder()
	NewHandler(cfg, "x").ServeHTTP(rec, httptest.NewRequest("GET", "/otra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
}
