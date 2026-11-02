package cfgfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/config"
)

func TestSaveRoundtripAndBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("routes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n"), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps = append(cfg.Apps, config.App{Name: "w", Command: "x", RestartDelay: "5s"})
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save1: %v", err)
	}
	// segundo save crea backup
	cfg.Routes = append(cfg.Routes, config.Route{Hosts: []string{"b.com"}, Target: "http://127.0.0.1:2"})
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save2: %v", err)
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) < 1 {
		t.Fatalf("quiere >=1 backup, got %v", backups)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "b.com") || !strings.Contains(string(data), "name: w") {
		t.Fatalf("contenido incompleto:\n%s", data)
	}
	re, err := Load(path)
	if err != nil || len(re.Apps) != 1 || len(re.Routes) != 2 {
		t.Fatalf("recarga mal: %+v %v", re, err)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("routes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n"), 0o644)
	cfg, _ := Load(path)
	cfg.Routes = append(cfg.Routes, config.Route{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:2"})
	if err := Save(path, cfg); err == nil {
		t.Fatal("duplicado debe rechazarse sin escribir")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "127.0.0.1:2") {
		t.Fatal("archivo tocado pese al rechazo")
	}
}
