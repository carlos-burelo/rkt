package mcpserver

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"rkt/internal/cfgfile"
	"rkt/internal/config"
	"rkt/internal/logx"
	"rkt/internal/store"
)

// Flujo MCP real contra Postgres local (puerta: RKT_PG_TEST_DSN).
// En CI se salta.
func TestPgMcpLive(t *testing.T) {
	if os.Getenv("RKT_PG_TEST_DSN") == "" {
		t.Skip("sin RKT_PG_TEST_DSN")
	}
	pgBin := `C:\Program Files\PostgreSQL\18\bin\pg_ctl.exe`
	if _, err := os.Stat(pgBin); err != nil {
		t.Skipf("sin postgres local: %v", err)
	}
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for k, v := range map[string]string{
		"pg_mode": "local", "pg_bin": pgBin,
		"pg_pass": os.Getenv("RKT_PG_TEST_PASS"),
	} {
		if err := db.SetSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	// La base cuelga de una app existente: dar de alta "w" en la flota.
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		cp.Apps = append(cp.Apps, config.App{Name: "w", Command: "c", Dir: t.TempDir(), RestartDelay: "5s"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.doPgProvision(pgProvisionArg{App: "w", Mode: "shared"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if !strings.Contains(out, `"mode":"shared"`) || !strings.Contains(out, `"url":"postgres://rkt_apps@`) {
		t.Fatalf("out: %s", out)
	}
	out, err = s.doPgList()
	if err != nil || !strings.Contains(out, `"app":"w"`) {
		t.Fatalf("list: %s %v", out, err)
	}
	if _, err := s.doPgDrop(pgDropArg{App: "w"}); err == nil {
		t.Fatal("drop sin confirm debe fallar")
	}
	if _, err := s.doPgDrop(pgDropArg{App: "w", Confirm: "drop"}); err != nil {
		t.Fatalf("drop: %v", err)
	}
}
