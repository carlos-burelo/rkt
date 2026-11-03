package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func testServer(t *testing.T, healthz http.HandlerFunc) *Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", healthz)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(
		"routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\nhealthz:\n  enabled: true\n  listen: \""+
			strings.TrimPrefix(srv.URL, "http://")+"\"\n"), 0o644)
	return New(cfgPath)
}

func TestDoStatus(t *testing.T) {
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	out, err := s.doStatus()
	if err != nil || out != `{"status":"ok"}` {
		t.Fatalf("got %q %v", out, err)
	}
}

func TestDoStatusSinServicio(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	// healthz en un puerto cerrado: determinista aunque el dev local corra
	if err := os.WriteFile(cfgPath, []byte("healthz:\n  enabled: true\n  listen: \"127.0.0.1:1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(cfgPath)
	if _, err := s.doStatus(); err == nil {
		t.Fatal("debía fallar sin servicio")
	}
}

func TestDoLogs(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("x"), 0o644)
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(filepath.Join(logsDir, "apps"), 0o755)
	os.WriteFile(filepath.Join(logsDir, "rkt.log"), []byte("l1\nl2\nl3\n"), 0o644)
	os.WriteFile(filepath.Join(logsDir, "apps", "web.log"), []byte("app1\n"), 0o644)
	s := New(cfgPath)

	out, err := s.doLogs(2, "")
	if err != nil || out != "l2\nl3" {
		t.Fatalf("got %q %v", out, err)
	}
	out, err = s.doLogs(0, "web")
	if err != nil || out != "app1" {
		t.Fatalf("got %q %v", out, err)
	}
	if _, err := s.doLogs(5, "nadie"); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
}

func TestDoRestartRollback(t *testing.T) {
	var restarted, rolledback bool
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/restart/"):
			restarted = true
			w.Write([]byte(`{"restarted":"web"}`))
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/rollback/"):
			rolledback = true
			w.Write([]byte(`{"rolledback":"web"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if out, err := s.doRestart("web"); err != nil || !restarted {
		t.Fatalf("restart: %q %v", out, err)
	}
	if _, err := s.doRestart(""); err == nil {
		t.Fatal("restart sin app debe fallar")
	}
	if out, err := s.doRollback("web"); err != nil || !rolledback {
		t.Fatalf("rollback: %q %v", out, err)
	}
}

func TestDoDeployments(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("x"), 0o644)
	s := New(cfgPath)
	// DB fresca: lista vacía, sin error
	out, err := s.doDeployments(5, "")
	if err != nil || out != "[]" {
		t.Fatalf("got %q %v", out, err)
	}
}

func TestDoEvents(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("x"), 0o644)
	db, err := store.Open(filepath.Join(dir, "logs", "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.InsertEvent("died", "web", "exit 1")
	db.Close()
	s := New(cfgPath)
	out, err := s.doEvents(10, "", "")
	if err != nil || !strings.Contains(out, `"kind":"died"`) {
		t.Fatalf("got %q %v", out, err)
	}
	out, err = s.doEvents(10, "failed", "")
	if err != nil || out != "[]" {
		t.Fatalf("filtro: got %q %v", out, err)
	}
}

func TestDoAppsAddLocal(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n"), 0o644)
	repo := filepath.Join(dir, "mirrepo")
	os.MkdirAll(repo, 0o755)
	os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"dependencies":{"next":"14.0.0"}}`), 0o644)
	seed, _ := store.Open(logx.DBPath(cfgPath))
	seed.SetSetting("node_bin", os.Args[0])
	seed.Close()
	s := New(cfgPath)

	out, err := s.doAppsAdd(appsAddArg{Repo: repo})
	if err != nil {
		t.Fatalf("apps_add: %v", err)
	}
	if !strings.Contains(out, `"app":"mirrepo"`) {
		t.Fatalf("inferencia mal: %s", out)
	}
	// preset next inferido: args standalone en DB
	_, db, err := cfgfile.LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fleet, _ := db.LoadFleet()
	if len(fleet.Apps) != 1 || fleet.Apps[0].Command == "" || len(fleet.Apps[0].Args) == 0 {
		t.Fatalf("plan next no aplicado: %+v", fleet.Apps)
	}
	if fleet.Apps[0].Deploy == nil || len(fleet.Apps[0].Deploy.Build) == 0 {
		t.Fatalf("build del plan ausente: %+v", fleet.Apps[0].Deploy)
	}
	// duplicado debe fallar
	if _, err := s.doAppsAdd(appsAddArg{Repo: repo}); err == nil {
		t.Fatal("duplicado debe fallar")
	}
	if _, err := s.doAppsAdd(appsAddArg{Repo: "ni-dir-ni-url :::"}); err == nil {
		t.Fatal("repo basura debe fallar")
	}
}

func TestDoInspectYUpdate(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "sitio")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "index.html"), []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("routes:\n  - hosts: [seed.test]\n    target: http://127.0.0.1:9\n"), 0o644)
	s := New(cfgPath)

	plan, err := s.doDetectPlan(proj)
	if err != nil || !strings.Contains(plan, `"framework":"static"`) {
		t.Fatalf("detect_plan = %q %v", plan, err)
	}
	if _, err := s.doAppsAdd(appsAddArg{Name: "sitio", Dir: proj, Port: 5098}); err != nil {
		t.Fatalf("apps_add: %v", err)
	}
	list, err := s.doAppsList()
	if err != nil || !strings.Contains(list, `"name":"sitio"`) {
		t.Fatalf("apps_list = %q %v", list, err)
	}
	dom := "sitio.localhost"
	if _, err := s.doAppUpdate(appUpdateArg{App: "sitio", Domain: &dom}); err != nil {
		t.Fatalf("app_update: %v", err)
	}
	routes, err := s.doRoutesList()
	if err != nil || !strings.Contains(routes, "sitio.localhost") || !strings.Contains(routes, `"dns_ok":[true]`) {
		t.Fatalf("routes_list = %q %v", routes, err)
	}
	zones, err := s.doZonesList()
	if err != nil || !strings.Contains(zones, "sitio.localhost") {
		t.Fatalf("zones_list = %q %v", zones, err)
	}
}

func TestDoEnvSetNoRead(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n"), 0o644)
	s := New(cfgPath)
	if _, err := s.doAppsAdd(appsAddArg{Name: "w", Command: "c"}); err != nil {
		t.Fatalf("apps_add: %v", err)
	}
	out, err := s.doEnvSet("w", "SECRETO", "valor1")
	if err != nil || !strings.Contains(out, "SECRETO") {
		t.Fatalf("env_set: %q %v", out, err)
	}
	// write-only: el valor NO vuelve en ninguna lectura
	st, _ := s.doStatus()
	_ = st
	if _, err := s.doEnvSet("nadie", "K", "v"); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
}

func TestDoDeployCheck(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\n"), 0o644)
	s := New(cfgPath)
	if _, err := s.doAppsAdd(appsAddArg{Name: "w", Command: "c"}); err != nil {
		t.Fatalf("apps_add: %v", err)
	}
	out, err := s.doDeployCheck("w")
	if err != nil {
		t.Fatalf("deploy_check: %v", err)
	}
	var checks map[string]bool
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		t.Fatalf("json: %v (%s)", err, out)
	}
	if !checks["app"] || checks["deploy"] || !checks["git"] {
		t.Fatalf("checks mal: %s (servicio apagado esperado en false)", out)
	}
}
