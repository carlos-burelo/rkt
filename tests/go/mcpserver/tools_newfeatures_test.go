package mcpserver

// Tools nuevas por el flujo real del gate MCP: canary, credenciales git,
// canales de correo/webhook, guía DNS y mapa de ataques.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"rkt/internal/canary"
	"rkt/internal/logx"
	"rkt/internal/store"
)

func okTool(t *testing.T, s *Server, tok, tool string, args map[string]any) string {
	t.Helper()
	code, body := callTool(t, s, tok, tool, args)
	if code != http.StatusOK {
		t.Fatalf("%s: %d %s", tool, code, body)
	}
	return body
}

func fakeExe() []byte {
	switch runtime.GOOS {
	case "windows":
		return []byte("MZ\x90\x00dev")
	case "linux":
		return []byte{0x7f, 'E', 'L', 'F', 'd'}
	}
	return []byte{0xcf, 0xfa, 0xed, 0xfe, 'd'}
}

func TestCanaryTools(t *testing.T) {
	s, toks := gateServer(t)
	adm, op := toks[store.RoleAdmin], toks[store.RoleOperator]

	if body := okTool(t, s, op, "canary_list", map[string]any{}); !strings.Contains(body, "[]") {
		t.Fatalf("lista vacía: %s", body)
	}
	if body := okTool(t, s, op, "canary_status", map[string]any{}); !strings.Contains(body, "version") || !strings.Contains(body, "canaries") {
		t.Fatalf("status: %s", body)
	}
	// operator no sube/aplica/descarta
	for _, tool := range []string{"canary_fetch", "canary_apply", "canary_discard", "canary_rollback"} {
		if code, _ := callTool(t, s, op, tool, map[string]any{}); code != http.StatusForbidden {
			t.Errorf("operator %s: %d, quiere 403", tool, code)
		}
	}
	// admin: validaciones antes de tocar red o procesos
	for tool, args := range map[string]map[string]any{
		"canary_fetch":   {"url": "http://x.com/rkt.exe", "sha256": strings.Repeat("a", 64)},
		"canary_apply":   {"id": "20260101-000000-aaaaaaaa"},
		"canary_discard": {"id": "../x"},
	} {
		_, body := callTool(t, s, adm, tool, args)
		if !strings.Contains(body, "isError") && !strings.Contains(strings.ToLower(body), "inv") && !strings.Contains(body, "inexistente") {
			t.Errorf("%s debía rechazar: %s", tool, body)
		}
	}
	// ciclo real: subido -> listado -> borrado
	st := canary.Open(logx.HomeDir(s.cfgPath))
	m, err := st.Put(strings.NewReader(string(fakeExe())), "dev", "", "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if body := okTool(t, s, op, "canary_list", map[string]any{}); !strings.Contains(body, m.ID) {
		t.Fatalf("falta el canary subido: %s", body)
	}
	okTool(t, s, adm, "canary_discard", map[string]any{"id": m.ID})
	if len(st.List()) != 0 {
		t.Fatal("canary_discard no borró")
	}
}

func TestGitTools(t *testing.T) {
	s, toks := gateServer(t)
	adm, op := toks[store.RoleAdmin], toks[store.RoleOperator]

	// SSH generada en el servidor: pública en la respuesta, privada nunca
	body := okTool(t, s, adm, "git_cred_add", map[string]any{"name": "web", "kind": "ssh", "match": "acme/web"})
	if !strings.Contains(body, "ssh-ed25519") || strings.Contains(body, "PRIVATE KEY") {
		t.Fatalf("add ssh: %s", body)
	}
	if _, err := os.Stat(filepath.Join(logx.SecretsDir(s.cfgPath), "git", "web.key")); err != nil {
		t.Fatal("falta la privada en secrets:", err)
	}
	if _, b := callTool(t, s, adm, "git_cred_add", map[string]any{"name": "web", "kind": "ssh"}); !strings.Contains(b, "ya existe") {
		t.Fatalf("duplicado: %s", b)
	}
	if code, _ := callTool(t, s, op, "git_cred_add", map[string]any{"name": "x", "kind": "ssh"}); code != http.StatusForbidden {
		t.Fatalf("operator crea credencial: %d", code)
	}
	if body := okTool(t, s, op, "git_creds_list", map[string]any{}); !strings.Contains(body, `\"web\"`) && !strings.Contains(body, `"web"`) {
		t.Fatalf("lista: %s", body)
	}
	// acceso real contra un repo local (sin auth): mecanismo ssh-url + ls-remote
	if _, err := exec.LookPath("git"); err == nil {
		bare := filepath.Join(t.TempDir(), "r.git")
		if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v %s", err, out)
		}
		if b := okTool(t, s, op, "git_access_check", map[string]any{"repo": bare}); !strings.Contains(b, "ssh-url") {
			t.Fatalf("access ok: %s", b)
		}
		if b := okTool(t, s, op, "git_access_check", map[string]any{"repo": filepath.Join(bare, "no")}); !strings.Contains(b, "hint") {
			t.Fatalf("access falla con pista: %s", b)
		}
	}
	if _, b := callTool(t, s, op, "git_access_check", map[string]any{"repo": ""}); !strings.Contains(b, "falta repo") {
		t.Fatalf("repo vacío: %s", b)
	}
	// sin GitHub App configurada: error accionable
	if _, b := callTool(t, s, op, "github_revalidate", map[string]any{}); !strings.Contains(b, "no configurada") {
		t.Fatalf("revalidate sin App: %s", b)
	}
	okTool(t, s, adm, "git_cred_remove", map[string]any{"name": "web"})
	if _, b := callTool(t, s, adm, "git_cred_remove", map[string]any{"name": "web"}); !strings.Contains(b, "inexistente") {
		t.Fatalf("borrar dos veces: %s", b)
	}
}

func TestAlertChannelTools(t *testing.T) {
	s, toks := gateServer(t)
	op := toks[store.RoleOperator]
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer hook.Close()

	if _, b := callTool(t, s, op, "alert_channels_add", map[string]any{"name": "mail", "url": "mailto:ops@example.com"}); !strings.Contains(b, "SMTP sin configurar") {
		t.Fatalf("mailto sin SMTP: %s", b)
	}
	if b := okTool(t, s, op, "alert_channels_add", map[string]any{"name": "hook", "url": hook.URL}); !strings.Contains(b, "alert_channel_test") {
		t.Fatalf("add webhook: %s", b)
	}
	if b := okTool(t, s, op, "alert_channel_test", map[string]any{"id": 1}); !strings.Contains(b, "ok") {
		t.Fatalf("test: %s", b)
	}
	if _, b := callTool(t, s, op, "alert_channel_test", map[string]any{"id": 99}); !strings.Contains(b, "inexistente") {
		t.Fatalf("test inexistente: %s", b)
	}
	okTool(t, s, op, "alert_channel_remove", map[string]any{"id": 1})
	if _, b := callTool(t, s, op, "alert_channel_remove", map[string]any{"id": 1}); !strings.Contains(b, "inexistente") {
		t.Fatalf("borrar dos veces: %s", b)
	}
}

func TestDomainGuideYAtaques(t *testing.T) {
	s, toks := gateServer(t)
	op := toks[store.RoleOperator]
	if b := okTool(t, s, op, "domain_guide", map[string]any{"host": "app.localhost"}); !strings.Contains(b, "local") {
		t.Fatalf("guía local: %s", b)
	}
	if _, b := callTool(t, s, op, "domain_guide", map[string]any{"host": "a b"}); !strings.Contains(b, "inválido") {
		t.Fatalf("host inválido: %s", b)
	}
	if b := okTool(t, s, op, "attacks_summary", map[string]any{"hours": 48}); !strings.Contains(b, "geoip_ready") {
		t.Fatalf("attacks: %s", b)
	}
}
