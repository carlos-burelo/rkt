package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/gitcred"
	"rkt/internal/store"
)

func post(h http.Handler, path, tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Credenciales git: alta solo admin, la lista jamás trae secretos, la llave
// SSH se genera en el servidor y el borrado limpia el secreto.
func TestGitCredsAPI(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	home := t.TempDir()
	d.CfgPath = filepath.Join(home, "rkt.db")
	h := NewHandler(d)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)
	op := loginAs(t, h, db, "op", store.RoleOperator)
	adm := loginAs(t, h, db, "adm", store.RoleAdmin)

	pat := "ghp_" + strings.Repeat("s", 36)
	body := `{"name":"ci","kind":"pat","match":"acme/*","secret":"` + pat + `"}`
	if rec := post(h, "/api/v1/git/credentials", op, body); rec.Code != http.StatusForbidden {
		t.Fatalf("operator crea: %d, quiere 403", rec.Code)
	}
	if rec := post(h, "/api/v1/git/credentials", adm, body); rec.Code != http.StatusCreated {
		t.Fatalf("admin crea pat: %d %s", rec.Code, rec.Body.String())
	}
	rec := post(h, "/api/v1/git/credentials", adm, `{"name":"deploy-web","kind":"ssh","match":"acme/web"}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "ssh-ed25519 ") {
		t.Fatalf("ssh generada: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "PRIVATE KEY") {
		t.Fatal("la privada no puede viajar en la respuesta")
	}
	if rec := post(h, "/api/v1/git/credentials", adm, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicado: %d, quiere 400", rec.Code)
	}

	if rec := httptest.NewRecorder(); true {
		h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/git/credentials", vis))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("viewer lista: %d, quiere 403", rec.Code)
		}
	}
	list := httptest.NewRecorder()
	h.ServeHTTP(list, authed(http.MethodGet, "/api/v1/git/credentials", op))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), pat) || strings.Contains(list.Body.String(), "PRIVATE KEY") ||
		!strings.Contains(list.Body.String(), `"deploy-web"`) {
		t.Fatalf("lista: %d %s", list.Code, list.Body.String())
	}

	secret := filepath.Join(home, "secrets", "git", "ci.pat")
	if _, err := os.Stat(secret); err != nil {
		t.Fatal("falta el secreto en disco:", err)
	}
	if got := gitcred.Pick(mustList(t, db), "acme", "web"); got == nil || got.Name != "deploy-web" {
		t.Fatalf("pick: %+v", got)
	}

	del := httptest.NewRecorder()
	h.ServeHTTP(del, authed(http.MethodDelete, "/api/v1/git/credentials/ci", op))
	if del.Code != http.StatusForbidden {
		t.Fatalf("operator borra: %d, quiere 403", del.Code)
	}
	del = httptest.NewRecorder()
	h.ServeHTTP(del, authed(http.MethodDelete, "/api/v1/git/credentials/ci", adm))
	if del.Code != http.StatusOK {
		t.Fatalf("admin borra: %d %s", del.Code, del.Body.String())
	}
	if _, err := os.Stat(secret); !os.IsNotExist(err) {
		t.Fatal("el secreto debe borrarse")
	}
}

func mustList(t *testing.T, db *store.DB) []store.GitCred {
	t.Helper()
	l, err := db.ListGitCreds()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Revalidate sin App configurada: error accionable, no pánico; con rol correcto.
func TestRevalidateSinApp(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)
	op := loginAs(t, h, db, "op", store.RoleOperator)
	if rec := post(h, "/api/v1/github/revalidate", vis, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", rec.Code)
	}
	// sin cableado (nil) responde 501, no cae
	if rec := post(h, "/api/v1/github/revalidate", op, ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("sin cablear: %d", rec.Code)
	}
	d.GitResolve = nil
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/git/access?repo=https://github.com/a/b", op))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("access sin cablear: %d", rec.Code)
	}
}
