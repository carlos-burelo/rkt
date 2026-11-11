package mcpserver

// Puerta MCP a nivel HTTP: cada rol ve lo suyo, las mutaciones se
// auditan y lo desconocido exige admin.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func gateServer(t *testing.T) (*Server, map[string]string) {
	t.Helper()
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	toks := map[string]string{}
	for _, u := range []struct{ name, role string }{
		{"adm", store.RoleAdmin},
		{"op", store.RoleOperator},
		{"vis", store.RoleViewer},
	} {
		if err := db.CreateUser(u.name, "secreto-largo-123", u.role); err != nil {
			t.Fatal(err)
		}
		tok, err := db.CreateToken(u.name, u.role, 24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		toks[u.role] = tok
	}
	return s, toks
}

func callTool(t *testing.T, s *Server, tok, tool string, args map[string]any) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	s.HTTPHandler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestGatePorRol(t *testing.T) {
	s, toks := gateServer(t)

	// viewer lee (domains_suggest no toca backend): 200.
	if code, body := callTool(t, s, toks[store.RoleViewer], "domains_suggest", map[string]any{"name": "x"}); code != 200 || strings.Contains(body, "requiere rol") {
		t.Fatalf("viewer lectura: %d %s", code, body)
	}
	// viewer no muta ni administra.
	for _, tool := range []string{"env_set", "routes_add", "deploy", "users_add", "config_import"} {
		code, body := callTool(t, s, toks[store.RoleViewer], tool, map[string]any{})
		if code != http.StatusForbidden || !strings.Contains(body, "requiere rol") {
			t.Fatalf("viewer %s: %d %s", tool, code, body)
		}
	}
	// operator muta pero no administra usuarios.
	if code, body := callTool(t, s, toks[store.RoleOperator], "env_global_set", map[string]any{"key": "G", "value": "1"}); code != 200 {
		t.Fatalf("operator env_global_set: %d %s", code, body)
	}
	for _, tool := range []string{"users_add", "config_import", "prune"} {
		code, body := callTool(t, s, toks[store.RoleOperator], tool, map[string]any{})
		if code != http.StatusForbidden || !strings.Contains(body, "admin") {
			t.Fatalf("operator %s: %d %s", tool, code, body)
		}
	}
	// admin crea usuario de verdad (round-trip por el gate).
	if code, body := callTool(t, s, toks[store.RoleAdmin], "users_add", map[string]any{"name": "agente", "password": "secreto-largo-999", "role": "viewer"}); code != 200 {
		t.Fatalf("admin users_add: %d %s", code, body)
	}
	// herramienta desconocida: fail-closed (ni admin... no: admin sí pasa).
	if code, body := callTool(t, s, toks[store.RoleOperator], "futura_inexistente", map[string]any{}); code != http.StatusForbidden {
		t.Fatalf("desconocida operator: %d %s", code, body)
	}
	if code, _ := callTool(t, s, toks[store.RoleAdmin], "futura_inexistente", map[string]any{}); code == http.StatusForbidden {
		t.Fatal("admin debe pasar el gate (falla después, en el handler)")
	}
	// sin token no hay gate que valga (el mcpAuth de fuera da 401;
	// aquí el rol vacío no pasa de viewer).
	if code, _ := callTool(t, s, "", "users_add", map[string]any{}); code != http.StatusForbidden {
		t.Fatalf("sin token: %d", code)
	}
	// tools/list no es tools/call: pasa sin filtrar.
	listBody, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(listBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+toks[store.RoleViewer])
	rec := httptest.NewRecorder()
	s.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "deploy") {
		t.Fatalf("tools/list: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGateAuditaMutaciones(t *testing.T) {
	s, toks := gateServer(t)
	if code, body := callTool(t, s, toks[store.RoleOperator], "env_global_set", map[string]any{"key": "AUD", "value": "1"}); code != 200 {
		t.Fatalf("setup: %d %s", code, body)
	}
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.ListAudit(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range rows {
		if e.Actor == "op" && strings.HasPrefix(e.Action, "mcp ") {
			return
		}
	}
	b, _ := json.Marshal(rows)
	t.Fatalf("sin rastro de auditoría: %s", b)
}
