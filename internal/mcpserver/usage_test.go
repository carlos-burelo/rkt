package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvGlobalTools: alta, lista (con/sin secretos) y borrado idempotente.
func TestEnvGlobalTools(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	seedConfig(t, cfgPath, `{"routes":[]}`)
	s := New(cfgPath)

	if _, err := s.doEnvGlobalSet(envGlobalArg{Key: "API_URL", Value: "https://api.test"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, err := s.doEnvGlobalList(envGlobalListArg{})
	if err != nil || !strings.Contains(out, "API_URL") {
		t.Fatalf("list claves: %q %v", out, err)
	}
	if strings.Contains(out, "api.test") {
		t.Fatalf("sin include_secrets no debe salir el valor: %s", out)
	}
	out, err = s.doEnvGlobalList(envGlobalListArg{IncludeSecrets: true})
	if err != nil || !strings.Contains(out, "https://api.test") {
		t.Fatalf("list secretos: %q %v", out, err)
	}
	if _, err := s.doEnvGlobalDelete(envGlobalArg{Key: "API_URL"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.doEnvGlobalDelete(envGlobalArg{Key: "API_URL"}); err == nil {
		t.Fatal("borrar inexistente debe fallar")
	}
}

// TestDoAppUsage: mide el proceso real y respeta la ventana pedida.
func TestDoAppUsage(t *testing.T) {
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "index.html"), []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	healthz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","version":"v","routes":1,"apps":[{"name":"w","running":true,"pid":%d,"restarts":2,"http_ok":true}]}`, pid)
	}))
	defer healthz.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	body := fmt.Sprintf(`{"healthz":{"enabled":true,"listen":%q},"apps":[{"name":"w","command":"x","dir":%q,"env":{"PORT":"3000"}}],"routes":[{"hosts":["w.test"],"target":"http://127.0.0.1:3000"}]}`,
		strings.TrimPrefix(healthz.URL, "http://"), proj)
	seedConfig(t, cfgPath, body)
	s := New(cfgPath)
	out, err := s.doAppUsage(appUsageArg{App: "w", WindowS: 60})
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	var rep struct {
		App      string  `json:"app"`
		Pid      int     `json:"pid"`
		Running  bool    `json:"running"`
		RSSMB    float64 `json:"rss_mb"`
		Handles  uint32  `json:"handles"`
		Restarts int     `json:"restarts"`
		Disk     int64   `json:"disk_bytes"`
		Traffic  struct {
			WindowS int `json:"window_s"`
		} `json:"traffic"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("json: %v (%s)", err, out)
	}
	if rep.App != "w" || rep.Pid != pid || !rep.Running || rep.Restarts != 2 {
		t.Fatalf("reporte = %+v", rep)
	}
	if rep.RSSMB <= 0 || rep.Handles == 0 {
		t.Fatalf("métricas reales ausentes: %+v", rep)
	}
	if rep.Disk <= 0 {
		t.Fatalf("disco debe medir el dir: %+v", rep)
	}
	if rep.Traffic.WindowS != 60 {
		t.Fatalf("ventana = %d", rep.Traffic.WindowS)
	}
	if _, err := s.doAppUsage(appUsageArg{App: "nope"}); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
}

func TestAlertTools(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	seedConfig(t, cfgPath, `{"routes":[]}`)
	s := New(cfgPath)
	if _, err := s.doAlertsAdd(alertAddArg{App: "*", Kind: "failed", Threshold: 1, WindowMin: 60}); err != nil {
		t.Fatalf("add: %v", err)
	}
	enabled := false
	if _, err := s.doAlertsUpdate(alertUpdateArg{ID: 1, Enabled: &enabled, Kind: "latency_p95"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	out, err := s.doAlertsTest(1)
	if err != nil || !strings.Contains(out, "latency_p95") {
		t.Fatalf("test: %q %v", out, err)
	}
	if _, err := s.doAlertChannelsList(); err != nil {
		t.Fatalf("channels: %v", err)
	}
}
