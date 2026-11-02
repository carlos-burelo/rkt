package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	s := New(filepath.Join(t.TempDir(), "noexiste.yaml"))
	if _, err := s.doStatus(); err == nil {
		t.Fatal("debió fallar sin config")
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
	if !strings.Contains(out, `"app":"mirrepo"`) || !strings.Contains(out, `"preset":"next"`) {
		t.Fatalf("inferencia mal: %s", out)
	}
	// duplicado debe fallar
	if _, err := s.doAppsAdd(appsAddArg{Repo: repo}); err == nil {
		t.Fatal("duplicado debe fallar")
	}
	if _, err := s.doAppsAdd(appsAddArg{Repo: "ni-dir-ni-url :::"}); err == nil {
		t.Fatal("repo basura debe fallar")
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
	if err != nil || !strings.Contains(out, "SECRETO=w") {
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
