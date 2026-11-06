// Package hook: webhook GitHub -> auto-deploy instantáneo.
// POST /_rkt/hook/github con X-Hub-Signature-256 (HMAC del secreto).
// Solo eventos push a ramas trackeadas con auto:true. Responde 202
// y despliega en fondo (GitHub exige respuesta <10s).
package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
)

// Deps cablea el webhook.
type Deps struct {
	Live     *config.Live
	Secret   string
	Log      *slog.Logger
	DeployFn func(name, actor string)
	// OnInstall recibe eventos installation* (id, cuenta, acción).
	OnInstall func(id int64, account, action string)
}

// Handler sirve el webhook. deployFn(name, actor) hace deploy en fondo.
func NewHandler(d Deps) http.Handler {
	live, secret, deployFn, log := d.Live, d.Secret, d.DeployFn, d.Log
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("hook: panic recuperado", "v", v)
				http.Error(w, "error interno", http.StatusInternalServerError)
			}
		}()
		if r.URL.Path != "/_rkt/hook/github" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "solo POST", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "body inválido", http.StatusBadRequest)
			return
		}
		if !validSignature(r.Header.Get("X-Hub-Signature-256"), body, secret) {
			log.Warn("hook: firma inválida", "remote", r.RemoteAddr)
			http.Error(w, "firma inválida", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-GitHub-Event") != "push" {
			if id, account, action, ok := InstallationEvent(r, body); ok {
				if d.OnInstall != nil {
					d.OnInstall(id, account, action)
				}
				log.Info("hook: evento instalación", "action", action, "account", account)
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var ev struct {
			Ref        string `json:"ref"`
			Deleted    bool   `json:"deleted"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(body, &ev); err != nil || ev.Deleted {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
		matched := false
		for _, name := range matchApps(live.Get(), ev.Repository.FullName, branch) {
			matched = true
			log.Info("hook: push detectado, desplegando", "app", name, "rev", branch)
			go deployFn(name, "hook")
		}
		if !matched {
			log.Info("hook: push sin app (repo/rama no trackeados)",
				"repo", ev.Repository.FullName, "ref", ev.Ref)
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

// matchApps apps con auto:true cuyo repo/rama coinciden. SHA pineado nunca.
func matchApps(cfg *config.Config, fullName, branch string) []string {
	out := []string{}
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if a.Deploy == nil || !a.Deploy.Auto || branch == "" {
			continue
		}
		owner, repo, ok := github.ParseRepo(a.Deploy.Repo)
		if !ok {
			continue
		}
		if strings.EqualFold(owner+"/"+repo, fullName) && a.Deploy.Ref == branch {
			out = append(out, a.Name)
		}
	}
	return out
}

func validSignature(header string, body []byte, secret string) bool {
	if secret == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// SetupDeps cablea GET /_rkt/setup: flujo install (browser) y manifest.
type SetupDeps struct {
	Log *slog.Logger
	// Install: verifica instalación y la registra.
	Verify  func(id int64) (string, error)
	OnSetup func(id int64, account string) error
	// Manifest: canjea code y guarda credenciales. Devuelve install URL.
	Exchange   func(code string) (*github.ManifestCredentials, error)
	OnManifest func(c *github.ManifestCredentials) (string, error)
	// ConsumeState valida el state one-time del manifest. Sin él no se
	// guardan credenciales: el endpoint es público.
	ConsumeState func(state string) bool
	// DashboardURL destino tras instalar (vacío = "/").
	DashboardURL string
}

// SetupHandler atiende GET /_rkt/setup del flujo browser:
//   - setup_action=install: GitHub vuelve tras instalar; registra y vuelve al panel.
//   - setup_action=manifest&code=..: canjea credenciales y manda a instalar.
func SetupHandler(d SetupDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				d.Log.Error("setup: panic recuperado", "v", v)
				writeHookPage(w, http.StatusInternalServerError, "Error interno",
					`<h1>Algo falló</h1><p>El setup de GitHub no terminó. Revisa los logs del servicio.</p>`)
			}
		}()
		q := r.URL.Query()
		switch q.Get("setup_action") {
		case "install":
			setupInstall(w, r, d)
		case "manifest":
			setupManifest(w, r, d)
		default:
			// redirect de manifest: solo ?code= (sin setup_action)
			if q.Get("code") != "" {
				setupManifest(w, r, d)
				return
			}
			writeHookPage(w, http.StatusBadRequest, "Setup inválido",
				`<h1>Petición incompleta</h1><p>Falta <code>setup_action</code> o <code>code</code>.</p>`)
		}
	})
}

// hookCSS comparte el lenguaje visual del dashboard: fondo oscuro, borde
// fino, acento naranja y mono para datos. Sin frameworks ni assets.
const hookCSS = `
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;
background:#16181d;color:#e9eaec;font:14px/1.55 "Inter",ui-sans-serif,system-ui,sans-serif}
.card{width:100%;max-width:560px;border:1px solid rgba(255,255,255,.09);background:#1c1f25;padding:28px}
.brand{display:flex;align-items:center;gap:8px;margin-bottom:22px;font:600 12px/1 "JetBrains Mono",ui-monospace,Consolas,monospace;
letter-spacing:.14em;text-transform:uppercase;color:#9aa0aa}
.dot{width:7px;height:7px;background:#f08a2e}
h1{margin:0 0 10px;font-size:20px;line-height:1.2;letter-spacing:-.01em;font-weight:600}
p{margin:0 0 12px;color:#a7acb5}
b{color:#e9eaec;font-weight:600}
code{font-family:"JetBrains Mono",ui-monospace,Consolas,monospace;font-size:12px;color:#e9eaec;
background:rgba(255,255,255,.05);padding:1px 5px}
.btn,input[type=submit]{display:inline-block;margin-top:8px;padding:9px 16px;border:0;cursor:pointer;
background:#f08a2e;color:#1a1206;text-decoration:none;
font:600 11px/1 "JetBrains Mono",ui-monospace,Consolas,monospace;letter-spacing:.08em;text-transform:uppercase}
.btn:hover,input[type=submit]:hover{background:#ff9b45}
.note{margin-top:18px;padding-top:14px;border-top:1px solid rgba(255,255,255,.08);
font:11px/1.6 "JetBrains Mono",ui-monospace,Consolas,monospace;color:#7f8590}
.ok{color:#5fd0a0}
.err{border:1px solid rgba(226,104,90,.35);background:rgba(226,104,90,.06);color:#ef8a7c;
padding:9px 11px;font:11px/1.6 "JetBrains Mono",ui-monospace,Consolas,monospace;margin:0 0 12px}
`

func hookPage(title, body string) string {
	return `<!DOCTYPE html><html lang="es"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>rkt · ` + templateEscape(title) + `</title><style>` + hookCSS + `</style></head>` +
		`<body><main class="card"><div class="brand"><span class="dot"></span>rkt / github</div>` +
		body + `</main></body></html>`
}

func writeHookPage(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprint(w, hookPage(title, body))
}

func setupInstall(w http.ResponseWriter, r *http.Request, d SetupDeps) {
	q := r.URL.Query()
	var id int64
	if _, err := fmt.Sscanf(q.Get("installation_id"), "%d", &id); err != nil || id <= 0 {
		writeHookPage(w, http.StatusBadRequest, "Instalación inválida",
			`<h1>Instalación inválida</h1><p>GitHub no envió un <code>installation_id</code> válido.</p>`)
		return
	}
	account, err := d.Verify(id)
	if err != nil {
		d.Log.Warn("setup: instalación no verificada", "id", id, "err", err)
		detail := `<p>GitHub rechazó la verificación. Revisa que la App e instalación existan.</p>`
		if strings.Contains(err.Error(), "sin github app") {
			detail = `<p>Este servidor aún no tiene la clave de la App.</p>
<p>Completa primero la creación desde <b>Ajustes</b> y vuelve a intentar.</p>`
		}
		writeHookPage(w, http.StatusUnauthorized, "Sin verificar",
			`<h1>No pude verificar la instalación</h1><div class="err">La verificación contra GitHub falló.</div>`+detail)
		return
	}
	if err := d.OnSetup(id, account); err != nil {
		d.Log.Error("setup: registro fallido", "err", err)
		writeHookPage(w, http.StatusInternalServerError, "Registro fallido",
			`<h1>No pude guardar la instalación</h1><p>Revisa los logs del servicio e inténtalo otra vez.</p>`)
		return
	}
	d.Log.Info("setup: app vinculada", "account", account, "installation", id)
	target := d.DashboardURL
	if target == "" {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func setupManifest(w http.ResponseWriter, r *http.Request, d SetupDeps) {
	q := r.URL.Query()
	state := q.Get("state")
	if state == "" || d.ConsumeState == nil || !d.ConsumeState(state) {
		writeHookPage(w, http.StatusForbidden, "Setup no autorizado",
			`<h1>Setup no autorizado</h1><p>Falta el state del dashboard o ya caducó. Vuelve a <b>Ajustes</b> e inicia el flujo otra vez.</p>`)
		return
	}
	code := q.Get("code")
	if code == "" || d.Exchange == nil || d.OnManifest == nil {
		writeHookPage(w, http.StatusBadRequest, "Código faltante",
			`<h1>Falta el código</h1><p>GitHub no devolvió <code>code</code> en la URL.</p>`)
		return
	}
	creds, err := d.Exchange(code)
	if err != nil {
		d.Log.Warn("setup: canje manifest fallido", "err", err)
		writeHookPage(w, http.StatusUnauthorized, "Código inválido",
			`<h1>Código inválido o vencido</h1><p>Vuelve a <b>Ajustes</b> y crea la App otra vez.</p>`)
		return
	}
	installURL, err := d.OnManifest(creds)
	if err != nil {
		d.Log.Error("setup: guardar credenciales fallido", "err", err)
		writeHookPage(w, http.StatusInternalServerError, "Registro fallido",
			`<h1>No pude guardar la App</h1><p>Revisa los logs del servicio e inténtalo otra vez.</p>`)
		return
	}
	d.Log.Info("setup: app creada", "slug", creds.Slug, "id", creds.ID)
	http.Redirect(w, r, installURL, http.StatusSeeOther)
}

func templateEscape(s string) string {
	r := strings.ReplaceAll(s, "&", "&amp;")
	r = strings.ReplaceAll(r, "<", "&lt;")
	r = strings.ReplaceAll(r, ">", "&gt;")
	r = strings.ReplaceAll(r, `"`, "&quot;")
	return r
}

// InstallationEvent procesa eventos installation/installation_repositories:
// devuelve (installationID, account, acción) para refrescar discovery.
func InstallationEvent(r *http.Request, body []byte) (int64, string, string, bool) {
	ev := r.Header.Get("X-GitHub-Event")
	if ev != "installation" && ev != "installation_repositories" {
		return 0, "", "", false
	}
	var p struct {
		Action       string `json:"action"`
		Installation struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
			} `json:"account"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &p); err != nil || p.Installation.ID <= 0 {
		return 0, "", "", false
	}
	return p.Installation.ID, p.Installation.Account.Login, p.Action, true
}

// manifestTemplate form que POSTea el manifest a GitHub (auto-submit).
var manifestTemplate = template.Must(template.New("manifest").Parse(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>rkt · crear GitHub App</title><style>` + hookCSS + `</style></head>
<body><main class="card"><div class="brand"><span class="dot"></span>rkt / github</div>
<h1>Crear GitHub App {{.Name}}</h1>
<p>Te llevo a GitHub con el manifest listo. Allí pulsa <b>Create GitHub App</b>.</p>
<div class="err">No cierres esta pestaña: vuelve sola al terminar.</div>
<form action="{{.Target}}" method="post">
<input type="hidden" name="manifest" value="{{.Manifest}}">
<input type="submit" value="Continuar a GitHub">
</form>
<div class="note">permisos: contents read · metadata read<br>instalación: personal o cualquier organización donde seas admin</div>
<script>document.forms[0].submit()</script>
</main></body></html>`))

// NewAppFormHandler sirve GET /_rkt/new-app?name=&domain=[&org=]: formulario
// que crea la App vía manifest oficial (permisos mínimos + webhook + setup).
func NewAppFormHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		name, domain, org := q.Get("name"), q.Get("domain"), q.Get("org")
		if name == "" || domain == "" || q.Get("state") == "" {
			writeHookPage(w, http.StatusBadRequest, "Faltan datos",
				`<h1>Faltan datos</h1><p>Se requiere <code>?name=&amp;domain=&amp;state=</code>.</p>`)
			return
		}
		target := "https://github.com/settings/apps/new"
		if org != "" {
			target = "https://github.com/organizations/" + org + "/settings/apps/new"
		}
		manifest := manifestJSON(name, domain, q.Get("local") == "1", q.Get("tls") == "1", q.Get("state"))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = manifestTemplate.Execute(w, map[string]string{
			"Name": name, "Target": target, "Manifest": string(manifest),
		})
	})
}

// manifestJSON construye el manifest oficial. En local (localhost/127.0.0.1)
// GitHub no puede entregar webhooks: se omite hook_attributes y el deploy
// usa polling. En prod exige dominio público con TLS.
//
// public=true: una App privada solo puede instalarse en la cuenta dueña.
// Para cubrir repos de organizaciones donde el usuario es admin, la App
// debe ser pública (GitHub no permite elegir orgs en el manifest).
func manifestJSON(name, domain string, local, tls bool, state string) []byte {
	scheme := "https"
	if local && !tls {
		scheme = "http"
	}
	base := scheme + "://" + domain
	setup := base + "/_rkt/setup"
	if state != "" {
		setup += "?state=" + url.QueryEscape(state)
	}
	m := map[string]any{
		"name":         name,
		"description":  "Deploy PaaS rkt (auto TLS + supervisor). Acceso a todos los repositorios para clonar privados.",
		"url":          base,
		"redirect_url": setup,
		"setup_url":    setup,
		"public":       true,
		"default_permissions": map[string]string{
			"contents": "read", "metadata": "read",
		},
	}
	if !local {
		m["default_events"] = []string{"push"}
		m["hook_attributes"] = map[string]any{
			"url": base + "/_rkt/hook/github", "active": true,
		}
	}
	manifest, _ := json.Marshal(m)
	return manifest
}
