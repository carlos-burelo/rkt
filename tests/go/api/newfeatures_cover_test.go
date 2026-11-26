package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/canary"
	"rkt/internal/gitcred"
	"rkt/internal/github"
	"rkt/internal/store"
)

func TestGitAccessYRevalidateHandlers(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.GitResolve = func(repo string) (gitcred.Access, error) {
		if strings.Contains(repo, "roto") {
			return gitcred.Access{}, errors.New("repo no parseable")
		}
		return gitcred.Access{Via: "anonymous", Note: "sin App"}, nil
	}
	d.GithubRevalidate = func() (github.Report, error) {
		return github.Report{Healthy: true, Installations: []github.InstallStatus{{Account: "yo", Selection: "all"}}}, nil
	}
	h := NewHandler(d)
	op := loginAs(t, h, db, "op", store.RoleOperator)

	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, path, op))
		return rec.Code, rec.Body.String()
	}
	if code, _ := get("/api/v1/git/access"); code != http.StatusBadRequest {
		t.Fatalf("sin repo: %d", code)
	}
	if code, body := get("/api/v1/git/access?repo=https://roto"); code != http.StatusOK || !strings.Contains(body, `"ok":false`) {
		t.Fatalf("repo roto: %d %s", code, body)
	}
	if _, err := exec.LookPath("git"); err == nil {
		bare := filepath.Join(t.TempDir(), "r.git")
		if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		if _, body := get("/api/v1/git/access?repo=" + bare); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "anonymous") {
			t.Fatalf("acceso a repo local: %s", body)
		}
		_, body := get("/api/v1/git/access?repo=" + filepath.Join(bare, "no-existe"))
		if !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "GitHub App") {
			t.Fatalf("fallo con pista accionable: %s", body)
		}
	}
	if AccessHint("pat:x") == AccessHint("anonymous") || !strings.Contains(AccessHint("env"), "revalidate") {
		t.Fatal("la pista depende del mecanismo")
	}
	// método no permitido
	if rec := post(h, "/api/v1/git/access", op, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST access: %d", rec.Code)
	}

	rec := post(h, "/api/v1/github/revalidate", op, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"healthy":true`) {
		t.Fatalf("revalidate: %d %s", rec.Code, rec.Body.String())
	}
	d.GithubRevalidate = func() (github.Report, error) { return github.Report{}, errors.New("GitHub caído") }
	h = NewHandler(d)
	if rec := post(h, "/api/v1/github/revalidate", op, ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("error de GitHub: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/github/revalidate", op))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET revalidate: %d", rec.Code)
	}
}

func TestCanaryHandlersErrores(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.Canary = canary.Open(t.TempDir())
	h := NewHandler(d)
	adm := loginAs(t, h, db, "adm", store.RoleAdmin)

	for _, c := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/v1/canary/20260101-000000-aaaaaaaa/apply", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/canary/20260101-000000-aaaaaaaa", http.StatusNotFound},
		{http.MethodGet, "/api/v1/canary/rollback", http.StatusNotFound},
		{http.MethodPatch, "/api/v1/canary", http.StatusMethodNotAllowed},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(c.method, c.path, adm))
		if rec.Code != c.want {
			t.Errorf("%s %s: %d, quiere %d", c.method, c.path, rec.Code, c.want)
		}
	}
	// sin almacén cableado: 501, nunca pánico
	d.Canary = nil
	h = NewHandler(d)
	for _, p := range []string{"/api/v1/canary", "/api/v1/canary/rollback"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodGet, p, adm))
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s sin almacén: %d", p, rec.Code)
		}
	}
}

func TestGeoIPYCanalPrueba(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	op := loginAs(t, h, db, "op", store.RoleOperator)

	if rec := post(h, "/api/v1/security/geoip", op, ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("geoip sin cablear: %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/security/geoip", op))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET geoip: %d", rec.Code)
	}

	// prueba de canal: webhook real en httptest, y canal inexistente
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer hook.Close()
	if rec := post(h, "/api/v1/alert-channels", op, `{"name":"h","url":"`+hook.URL+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("crear canal: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(h, "/api/v1/alert-channels/1/test", op, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("probar canal: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(h, "/api/v1/alert-channels/99/test", op, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("canal inexistente: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/alert-channels/1/test", op))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET test: %d", rec.Code)
	}
}
