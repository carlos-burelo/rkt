package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
)

func sign(t *testing.T, body, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func liveEmpty(t *testing.T) *config.Live {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Routes = []config.Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"}}
	cfg.Apps = []config.App{{
		Name: "web", Command: "x", Dir: dir,
		Deploy: &config.Deploy{Repo: "https://github.com/o/r.git", Ref: "main", Auto: true},
	}}
	if err := cfg.Finalize(); err != nil {
		t.Fatal(err)
	}
	return config.NewLive(cfg)
}

func TestPushMatchDeploys(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mu sync.Mutex
	var deployed []string
	h := NewHandler(Deps{Live: liveEmpty(t), Secret: "s3cr3t", Log: log,
		DeployFn: func(name, actor string) {
			mu.Lock()
			deployed = append(deployed, name)
			mu.Unlock()
		}})

	body := `{"ref":"refs/heads/main","deleted":false,"repository":{"full_name":"o/r"}}`
	req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(t, body, "s3cr3t"))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code=%d", rec.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(deployed)
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("deployFn no corrió, deployed=%v", deployed)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if deployed[0] != "web" {
		t.Fatalf("deployed=%v", deployed)
	}
}

func TestPushIgnored(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	calls := 0
	h := NewHandler(Deps{Live: liveEmpty(t), Secret: "s3cr3t", Log: log,
		DeployFn: func(string, string) { calls++ }})
	signed := func(body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", sign(t, body, "s3cr3t"))
		req.Header.Set("X-GitHub-Event", "push")
		return req
	}

	cases := map[string]string{
		"otra rama":  `{"ref":"refs/heads/dev","deleted":false,"repository":{"full_name":"o/r"}}`,
		"otro repo":  `{"ref":"refs/heads/main","deleted":false,"repository":{"full_name":"o/otro"}}`,
		"borrado":    `{"ref":"refs/heads/main","deleted":true,"repository":{"full_name":"o/r"}}`,
		"json roto":  `{no-json`,
		"no es push": "",
	}
	for name, body := range cases {
		req := signed(body)
		if name == "no es push" {
			req.Header.Set("X-GitHub-Event", "ping")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s: code=%d", name, rec.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("nada debió desplegar, calls=%d", calls)
	}
}

func TestAuth(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(Deps{Live: liveEmpty(t), Secret: "s3cr3t", Log: log, DeployFn: func(string, string) {}})
	body := `{"ref":"refs/heads/main","deleted":false,"repository":{"full_name":"o/r"}}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin firma: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(t, body, "otra"))
	req.Header.Set("X-GitHub-Event", "push")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("firma mala: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/hook/github", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/otra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("otra ruta: code=%d", rec.Code)
	}
}

func TestSetup(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var gotID int64
	var gotAccount string
	h := SetupHandler(SetupDeps{Log: log, DashboardURL: "http://127.0.0.1:8090/settings?github=ok",
		Verify: func(id int64) (string, error) {
			if id != 42 {
				return "", fmt.Errorf("no")
			}
			return "miorg", nil
		},
		OnSetup: func(id int64, account string) error {
			gotID, gotAccount = id, account
			return nil
		}})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/setup?installation_id=42&setup_action=install", nil))
	if rec.Code != http.StatusSeeOther || gotID != 42 || gotAccount != "miorg" {
		t.Fatalf("setup ok: code=%d id=%d account=%q", rec.Code, gotID, gotAccount)
	}
	if loc := rec.Header().Get("Location"); loc != "http://127.0.0.1:8090/settings?github=ok" {
		t.Fatalf("location: %q", loc)
	}

	for url, code := range map[string]int{
		"/_rkt/setup?installation_id=0&setup_action=install": http.StatusBadRequest,
		"/_rkt/setup?installation_id=42&setup_action=update": http.StatusBadRequest,
		"/_rkt/setup?setup_action=install":                   http.StatusBadRequest,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != code {
			t.Fatalf("%s: code=%d", url, rec.Code)
		}
	}
}

func TestSetupVerifyFail(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	called := false
	h := SetupHandler(SetupDeps{Log: log,
		Verify:  func(int64) (string, error) { return "", fmt.Errorf("no") },
		OnSetup: func(int64, string) error { called = true; return nil }})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/setup?installation_id=7&setup_action=install", nil))
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatalf("code=%d called=%v", rec.Code, called)
	}
}

func TestInstallationEventFlow(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var gotID int64
	var gotAction string
	h := NewHandler(Deps{Live: liveEmpty(t), Secret: "s3cr3t", Log: log,
		DeployFn: func(string, string) {},
		OnInstall: func(id int64, account, action string) {
			gotID, gotAction = id, action
		}})
	body := `{"action":"created","installation":{"id":77,"account":{"login":"miorg"}}}`
	req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(t, body, "s3cr3t"))
	req.Header.Set("X-GitHub-Event", "installation")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || gotID != 77 || gotAction != "created" {
		t.Fatalf("code=%d id=%d action=%q", rec.Code, gotID, gotAction)
	}
}

func TestSetupManifest(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var savedSlug string
	h := SetupHandler(SetupDeps{Log: log,
		Exchange: func(code string) (*github.ManifestCredentials, error) {
			if code != "buen-code" {
				return nil, fmt.Errorf("mal code")
			}
			return &github.ManifestCredentials{ID: 5, Slug: "mi-app", WebhookSecret: "s", PEM: "k"}, nil
		},
		OnManifest: func(c *github.ManifestCredentials) (string, error) {
			savedSlug = c.Slug
			return "https://github.com/apps/" + c.Slug + "/installations/new", nil
		},
		ConsumeState: func(state string) bool { return state == "buen-state" }})

	// Sin state o con state inválido no se canjea el código.
	for _, u := range []string{
		"/_rkt/setup?setup_action=manifest&code=buen-code",
		"/_rkt/setup?setup_action=manifest&code=buen-code&state=mal-state",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: code=%d, quiere 403", u, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/setup?setup_action=manifest&code=buen-code&state=buen-state", nil))
	if rec.Code != http.StatusSeeOther || savedSlug != "mi-app" {
		t.Fatalf("code=%d slug=%q", rec.Code, savedSlug)
	}
	if loc := rec.Header().Get("Location"); loc != "https://github.com/apps/mi-app/installations/new" {
		t.Fatalf("location: %q", loc)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/setup?setup_action=manifest&code=mal-code&state=buen-state", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code malo: code=%d", rec.Code)
	}
}

func TestSetupManifestBareCode(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	called := false
	h := SetupHandler(SetupDeps{Log: log,
		Exchange: func(code string) (*github.ManifestCredentials, error) {
			if code != "abc123" {
				return nil, fmt.Errorf("mal code")
			}
			return &github.ManifestCredentials{ID: 1, Slug: "x", WebhookSecret: "s", PEM: "k"}, nil
		},
		OnManifest: func(c *github.ManifestCredentials) (string, error) {
			called = true
			return "https://github.com/apps/x/installations/new", nil
		},
		ConsumeState: func(state string) bool { return state == "st" }})
	// redirect real de GitHub: ?code= sin setup_action, con state del dashboard
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/setup?code=abc123&state=st", nil))
	if rec.Code != http.StatusSeeOther || !called {
		t.Fatalf("code pelado: code=%d called=%v", rec.Code, called)
	}
}

func TestNewAppForm(t *testing.T) {
	h := NewAppFormHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/new-app?name=mi-app&domain=vps.test&org=miorg&state=s1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"https://github.com/organizations/miorg/settings/apps/new",
		"push",
		"https://vps.test/_rkt/hook/github",
		"https://vps.test/_rkt/setup",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("form sin %q", want)
		}
	}
	for _, nope := range []string{"installation_repositories", "installation"} {
		if strings.Contains(body, nope) {
			t.Fatalf("form con evento inválido en manifest: %q", nope)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/new-app?name=&domain=&state=s1", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin params: code=%d", rec.Code)
	}
}

func TestManifestJSON(t *testing.T) {
	var m struct {
		Name               string            `json:"name"`
		URL                string            `json:"url"`
		RedirectURL        string            `json:"redirect_url"`
		SetupURL           string            `json:"setup_url"`
		Public             bool              `json:"public"`
		DefaultPermissions map[string]string `json:"default_permissions"`
		DefaultEvents      []string          `json:"default_events"`
		HookAttributes     map[string]any    `json:"hook_attributes"`
	}
	if err := json.Unmarshal(manifestJSON("mi-app", "vps.test", false, true, "st"), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "mi-app" || !m.Public || m.DefaultPermissions["contents"] != "read" {
		t.Fatalf("manifest mal: %+v", m)
	}
	// candado de regresión: GitHub rechaza installation* en manifest
	if len(m.DefaultEvents) != 1 || m.DefaultEvents[0] != "push" {
		t.Fatalf("eventos manifest mal: %v", m.DefaultEvents)
	}
	if m.RedirectURL != "https://vps.test/_rkt/setup?state=st" || m.SetupURL != m.RedirectURL {
		t.Fatalf("urls mal: %+v", m)
	}
	if m.HookAttributes["url"] != "https://vps.test/_rkt/hook/github" {
		t.Fatalf("hook mal: %+v", m.HookAttributes)
	}
}

func TestManifestLocalSinWebhook(t *testing.T) {
	var m struct {
		URL            string         `json:"url"`
		RedirectURL    string         `json:"redirect_url"`
		Public         bool           `json:"public"`
		DefaultEvents  []string       `json:"default_events"`
		HookAttributes map[string]any `json:"hook_attributes"`
	}
	if err := json.Unmarshal(manifestJSON("rkt-local", "127.0.0.1:8080", true, false, "st"), &m); err != nil {
		t.Fatal(err)
	}
	if m.URL != "http://127.0.0.1:8080" || m.RedirectURL != "http://127.0.0.1:8080/_rkt/setup?state=st" {
		t.Fatalf("urls locales mal: %+v", m)
	}
	if !m.Public {
		t.Fatal("la App debe ser pública para instalarse en organizaciones")
	}
	if m.HookAttributes != nil || len(m.DefaultEvents) != 0 {
		t.Fatalf("local no debe pedir webhook/eventos: %+v %v", m.HookAttributes, m.DefaultEvents)
	}
}
