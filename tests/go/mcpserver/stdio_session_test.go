package mcpserver

// Gate stdio + sesión en /mcp: el mismo rol decide en ambos transportes.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt/internal/logx"
	"rkt/internal/store"
)

func TestStdioGate(t *testing.T) {
	dir := t.TempDir()
	s := &Server{cfgPath: filepath.Join(dir, "rkt.db"), stdioGate: true}

	// Sin control.token: viewer (lee, no muta, no administra).
	if deny := s.stdioCheck("status"); deny != nil {
		t.Fatal("stdio viewer debería leer status")
	}
	if deny := s.stdioCheck("deploy"); deny == nil {
		t.Fatal("stdio viewer no debe mutar")
	}
	if deny := s.stdioCheck("users_add"); deny == nil {
		t.Fatal("stdio viewer no debe administrar")
	}
	if deny := s.stdioCheck("futura_inexistente"); deny == nil {
		t.Fatal("stdio desconocida fail-closed")
	}

	// Con control.token: local == operator (muta, no administra).
	if err := os.MkdirAll(logx.DirFor(s.cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logx.ControlTokenPath(s.cfgPath), []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if deny := s.stdioCheck("deploy"); deny != nil {
		t.Fatal("stdio local debería mutar")
	}
	if deny := s.stdioCheck("users_add"); deny == nil {
		t.Fatal("stdio local no debe administrar")
	}
}

func TestMcpAceptaSesion(t *testing.T) {
	s, toks := gateServer(t)
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sess, err := db.NewSession("op", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	callCookie := func(sess, tool string, origin string) (int, string) {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": tool, "arguments": map[string]any{}},
		})
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
		req.AddCookie(&http.Cookie{Name: "rkt_token", Value: sess})
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		s.gateHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"ok"}`))
		})).ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	// Sesión operator sin Origin: pasa como el Bearer.
	if code, _ := callCookie(sess, "restart", ""); code != 200 {
		t.Fatalf("sesión operator: %d", code)
	}
	// Sesión operator con Origin ajeno en mutación: se deniega (CSRF).
	if code, _ := callCookie(sess, "restart", "https://evil.test"); code != http.StatusForbidden {
		t.Fatalf("origen ajeno: %d", code)
	}
	// Sesión viewer no muta aunque el Origin sea propio.
	if code, _ := callCookie(toks[store.RoleViewer], "restart", ""); code != http.StatusForbidden {
		t.Fatalf("sesión viewer mutando: %d", code)
	}
}
