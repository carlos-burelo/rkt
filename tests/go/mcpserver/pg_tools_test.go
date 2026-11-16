package mcpserver

import (
	"net/http"
	"strings"
	"testing"
)

func TestPgToolsSinPg(t *testing.T) {
	s := testServer(t, func(http.ResponseWriter, *http.Request) {})
	if _, err := s.doPgProvision(pgProvisionArg{}); err == nil || !strings.Contains(err.Error(), "app requerida") {
		t.Fatalf("sin app: %v", err)
	}
	if _, err := s.doPgProvision(pgProvisionArg{App: "zz"}); err == nil || !strings.Contains(err.Error(), "inexistente") {
		t.Fatalf("app inexistente: %v", err)
	}
	if _, err := s.doPgDrop(pgDropArg{}); err == nil || !strings.Contains(err.Error(), "app requerida") {
		t.Fatalf("sin app: %v", err)
	}
	if _, err := s.doPgDrop(pgDropArg{App: "x"}); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("sin confirm: %v", err)
	}
	if _, err := s.doPgBackup(pgAppArg{}); err == nil || !strings.Contains(err.Error(), "app requerida") {
		t.Fatalf("sin app: %v", err)
	}
	if _, err := s.doPgTables(pgTablesArg{}); err == nil || !strings.Contains(err.Error(), "app requerida") {
		t.Fatalf("tables sin app: %v", err)
	}
	if _, err := s.doPgRows(pgRowsArg{App: "x"}); err == nil || !strings.Contains(err.Error(), "requeridos") {
		t.Fatalf("rows sin tabla: %v", err)
	}
	if _, err := s.doPgQuery(pgQueryArg{App: "x", SQL: "SELECT 1"}); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("query sin confirm: %v", err)
	}
	if _, err := s.doPgRestore(pgRestoreArg{App: "x", File: "y"}); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("restore sin confirm: %v", err)
	}
	out, err := s.doPgList()
	if err != nil || !strings.Contains(out, `"enabled":false`) {
		t.Fatalf("sin pg debe salir desactivado: %s %v", out, err)
	}
	out, err = s.doPgStatus()
	if err != nil || !strings.Contains(out, `"enabled":false`) {
		t.Fatalf("status sin pg: %s %v", out, err)
	}
}
