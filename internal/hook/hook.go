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
}

// SetupHandler atiende GET /_rkt/setup del flujo browser:
//   - setup_action=install: GitHub vuelve tras instalar (verifica+registra).
//   - setup_action=manifest&code=..: canjea credenciales de App recién creada.
func SetupHandler(d SetupDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				d.Log.Error("setup: panic recuperado", "v", v)
				http.Error(w, "error interno", http.StatusInternalServerError)
			}
		}()
		q := r.URL.Query()
		if slug := q.Get("install"); slug != "" {
			installGuide(w, slug)
			return
		}
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
			http.Error(w, "solo install|manifest", http.StatusBadRequest)
		}
	})
}

// installGuide: paso entre crear la App e instalar. Insiste en "All
// repositories" (GitHub no permite forzarlo en el manifest).
var installGuideTpl = template.Must(template.New("install").Parse(`<!DOCTYPE html>
<html lang="es"><head><meta charset="utf-8"><title>rkt: instalar GitHub App</title></head>
<body style="font-family:system-ui;padding:48px;max-width:640px">
<h1>App creada. Ahora instálala</h1>
<p>En la página de GitHub elige <b>All repositories</b> (todos los repositorios).</p>
<p>Así rkt podrá clonar también repos privados sin PAT.</p>
<p><a href="https://github.com/apps/{{.Slug}}/installations/new"
 style="display:inline-block;padding:10px 18px;background:#18181b;color:#fff;text-decoration:none;border-radius:2px">
 Instalar con todos los repositorios →</a></p>
<p style="color:#71717a;font-size:13px">Si ya la instalaste con repos seleccionados,
cámbialo en Settings → Applications → rkt → Repository access.</p>
</body></html>`))

func installGuide(w http.ResponseWriter, slug string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = installGuideTpl.Execute(w, map[string]string{"Slug": slug})
}

func setupInstall(w http.ResponseWriter, r *http.Request, d SetupDeps) {
	q := r.URL.Query()
	var id int64
	if _, err := fmt.Sscanf(q.Get("installation_id"), "%d", &id); err != nil || id <= 0 {
		http.Error(w, "installation_id inválido", http.StatusBadRequest)
		return
	}
	account, err := d.Verify(id)
	if err != nil {
		d.Log.Warn("setup: instalación no verificada", "id", id, "err", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `<!DOCTYPE html><html lang="es"><head><meta charset="utf-8"><title>rkt sin verificar</title></head>
<body style="font-family:system-ui;padding:48px"><h1>No pude verificar la instalación</h1>`)
		if strings.Contains(err.Error(), "sin github app") {
			fmt.Fprint(w, `<p>Este servidor aún no tiene la clave de la App.</p>
<p>Completa primero la creación (<code>github init</code> → Create → recarga la
pestaña del código) y <b>reinicia el servicio</b> para cargarla.</p>`)
		} else {
			fmt.Fprint(w, `<p>GitHub rechazó la verificación. Revisa que la App e instalación existan.</p>`)
		}
		fmt.Fprint(w, `</body></html>`)
		return
	}
	if err := d.OnSetup(id, account); err != nil {
		d.Log.Error("setup: registro fallido", "err", err)
		http.Error(w, "registro fallido", http.StatusInternalServerError)
		return
	}
	d.Log.Info("setup: app vinculada", "account", account, "installation", id)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html lang="es"><head><meta charset="utf-8"><title>rkt vinculado</title></head>
<body style="font-family:system-ui;padding:48px"><h1>GitHub conectado ✓</h1>
<p>Cuenta <b>%s</b> vinculada. Los repos con despliegue se registran solos.</p>
<p>Puedes cerrar esta ventana.</p></body></html>`, templateEscape(account))
}

func setupManifest(w http.ResponseWriter, r *http.Request, d SetupDeps) {
	code := r.URL.Query().Get("code")
	if code == "" || d.Exchange == nil || d.OnManifest == nil {
		http.Error(w, "code requerido", http.StatusBadRequest)
		return
	}
	creds, err := d.Exchange(code)
	if err != nil {
		d.Log.Warn("setup: canje manifest fallido", "err", err)
		http.Error(w, "código inválido o vencido", http.StatusUnauthorized)
		return
	}
	installURL, err := d.OnManifest(creds)
	if err != nil {
		d.Log.Error("setup: guardar credenciales fallido", "err", err)
		http.Error(w, "registro fallido", http.StatusInternalServerError)
		return
	}
	d.Log.Info("setup: app creada", "slug", creds.Slug, "id", creds.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html lang="es"><head><meta charset="utf-8"><title>rkt app creada</title></head>
<body style="font-family:system-ui;padding:48px"><h1>App creada ✓</h1>
<p>Clave guardada en el servidor. Ahora <b>reinicia el servicio</b> para cargarla:</p>
<p><code>sc.exe stop Rkt; sc.exe start Rkt</code></p>
<p>Luego instala en tus repos:</p>
<p><a href="%s"><button>Instalar en GitHub</button></a></p></body></html>`, templateEscape(installURL))
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
<html lang="es"><head><meta charset="utf-8"><title>rkt: crear GitHub App</title></head>
<body style="font-family:system-ui;padding:48px">
<h1>Crear GitHub App {{.Name}}</h1>
<p>Revisa y pulsa <b>Create GitHub App</b> en GitHub (ya viene rellenado).</p>
<form action="{{.Target}}" method="post">
<input type="hidden" name="manifest" value="{{.Manifest}}">
<input type="submit" value="Continuar a GitHub">
</form>
<script>document.forms[0].submit()</script>
</body></html>`))

// NewAppFormHandler sirve GET /_rkt/new-app?name=&domain=[&org=]: formulario
// que crea la App vía manifest oficial (permisos mínimos + webhook + setup).
func NewAppFormHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		name, domain, org := q.Get("name"), q.Get("domain"), q.Get("org")
		if name == "" || domain == "" {
			http.Error(w, "requiere ?name=&domain=[&org=]", http.StatusBadRequest)
			return
		}
		target := "https://github.com/settings/apps/new"
		if org != "" {
			target = "https://github.com/organizations/" + org + "/settings/apps/new"
		}
		manifest := manifestJSON(name, domain, q.Get("local") == "1", q.Get("tls") == "1")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = manifestTemplate.Execute(w, map[string]string{
			"Name": name, "Target": target, "Manifest": string(manifest),
		})
	})
}

// manifestJSON construye el manifest oficial. En local (localhost/127.0.0.1)
// GitHub no puede entregar webhooks: se omite hook_attributes y el deploy
// usa polling. En prod exige dominio público con TLS.
func manifestJSON(name, domain string, local, tls bool) []byte {
	scheme := "https"
	if local && !tls {
		scheme = "http"
	}
	base := scheme + "://" + domain
	m := map[string]any{
		"name":         name,
		"description":  "Deploy PaaS rkt (auto TLS + supervisor). Acceso a todos los repositorios para clonar privados.",
		"url":          base,
		"redirect_url": base + "/_rkt/setup",
		"setup_url":    base + "/_rkt/setup",
		"public":       false,
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
