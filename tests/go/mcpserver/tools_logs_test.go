package mcpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/logx"
)

func TestLogsClearExigeConfirm(t *testing.T) {
	s := testServer(t, func(http.ResponseWriter, *http.Request) {})
	if _, err := s.doLogsClear(logsClearArg{}); err == nil || !strings.Contains(err.Error(), "confirm") {
		t.Fatalf("sin confirm debió fallar: %v", err)
	}
}

func TestLogsClearVacía(t *testing.T) {
	s := testServer(t, func(http.ResponseWriter, *http.Request) {})
	logs := logx.DirFor(s.cfgPath)
	body := "linea1\nlinea2\n"
	if err := os.MkdirAll(filepath.Join(logs, "apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "access.log"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "apps", "w.log"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.doLogsClear(logsClearArg{Confirm: "clear"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"bytes":28`) {
		t.Fatalf("debió reportar 28 bytes: %s", out)
	}
	for _, p := range []string{filepath.Join(logs, "access.log"), filepath.Join(logs, "apps", "w.log")} {
		if st, err := os.Stat(p); err != nil || st.Size() != 0 {
			t.Fatalf("%s no quedó vacío", p)
		}
	}
}
