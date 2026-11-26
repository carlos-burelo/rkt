//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"rkt/internal/github"
)

// El manifest de GitHub App se genera en el servidor, viaja en un form
// auto-submit y GitHub valida redirect_url (localhost+puerto lo rechaza).
// Sin navegador no se ve el HTML escapado ni el POST real: esta prueba va
// con Chromium hasta el POST, con github.com interceptado (sin red real),
// y valida el manifest capturado.
func TestGithubManifestRedirectURL(t *testing.T) {
	a := newBrowserApp(t)
	page, err := a.browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()

	// El setup_url lo arma la misma lógica pura de producción.
	hostport := strings.TrimPrefix(a.url, "http://")
	plan := github.PlanSetup("", "", hostport, "", nil)
	origin, domain := plan.Origin, plan.Domain
	if !plan.Local || plan.Env.Blocked() {
		t.Fatal("sin base_domain ni IP pública debe ser flujo local sin bloqueo")
	}
	setupURL := origin + "/_rkt/new-app?name=rkt-local-1&domain=" + url.QueryEscape(domain) +
		"&local=1&tls=0&state=e2estate"

	var postBody string
	var postMethod string
	if err := page.Route("**/github.com/**", func(route playwright.Route) {
		req := route.Request()
		postMethod = req.Method()
		if body, err := req.PostData(); err == nil {
			postBody = body
		}
		_ = route.Fulfill(playwright.RouteFulfillOptions{
			Status:      playwright.Int(200),
			ContentType: playwright.String("text/html"),
			Body:        "<html><body>stub-github-ok</body></html>",
		})
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := page.Goto(setupURL); err != nil {
		t.Fatalf("form new-app: %v", err)
	}
	// El auto-submit del form navega al stub de github.com.
	if err := page.WaitForURL("**/github.com/**"); err != nil {
		t.Fatalf("el auto-submit no navegó a github: %v", err)
	}
	content, err := page.Content()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "stub-github-ok") {
		t.Fatalf("el stub no respondió al POST: %s", content[:200])
	}
	if postMethod != "POST" {
		t.Fatalf("method = %q, quiere POST", postMethod)
	}
	vals, err := url.ParseQuery(postBody)
	if err != nil {
		t.Fatalf("POST no es form: %v", err)
	}
	raw := vals.Get("manifest")
	if raw == "" {
		t.Fatalf("sin campo manifest en el POST: %q", postBody)
	}
	var manifest map[string]any
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		t.Fatalf("manifest no es JSON (¿doble escape del template?): %v", err)
	}
	redir, _ := manifest["redirect_url"].(string)
	want := origin + "/_rkt/setup?state=e2estate"
	if redir != want {
		t.Fatalf("redirect_url = %q, quiere %q", redir, want)
	}
	if strings.Contains(redir, "localhost:") {
		t.Fatalf("localhost con puerto: GitHub lo rechaza: %q", redir)
	}
	u, err := url.Parse(redir)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		t.Fatalf("redirect_url inválida: %q", redir)
	}
}
