package app

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"rkt/internal/gitcred"
	"rkt/internal/github"
	"rkt/internal/logx"
	"rkt/internal/store"
)

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// verifyInstallation confirma una instalación contra la API (setup browser).
// Incluye reintentos con backoff suave ante latencia de propagación de réplicas en GitHub.
func (p *Program) verifyInstallation(id int64) (string, error) {
	if p.gh() == nil {
		return "", fmt.Errorf("sin github app")
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		account, err := p.gh().Installation(id)
		if err == nil {
			return account, nil
		}
		lastErr = err
		time.Sleep(1 * time.Second)
	}
	return "", lastErr
}

// onSetup atiende el setup del browser tras instalar: verifica y dispara
// discovery (la API es la fuente; nada que persistir).
func (p *Program) onSetup(id int64, account string) error {
	p.log.Info("setup: instalación confirmada", "account", account, "installation", id)
	if p.gh() != nil {
		info, err := p.gh().InstallationDetail(id)
		if err != nil {
			p.log.Warn("setup: no pude leer la instalación", "err", err)
		} else {
			if info.Account != "" {
				account = info.Account
			}
			settings := map[string]string{
				"github_install_id":     strconv.FormatInt(id, 10),
				"github_account":        account,
				"github_repo_selection": info.RepositorySelection,
				"github_install_url":    info.HTMLURL,
				"github_install_target": info.TargetType,
			}
			for k, v := range settings {
				if err := p.db.SetSetting(k, v); err != nil {
					return err
				}
			}
			if tok, err := p.gh().TokenFor(id); err == nil {
				if repos, err := p.gh().Repositories(tok); err == nil {
					_ = p.db.SetSetting("github_repo_count", strconv.Itoa(len(repos)))
				}
			}
			if info.RepositorySelection != "all" {
				p.log.Warn("github: instalación con repos seleccionados; activa All repositories para clonar privados",
					"account", account, "settings", info.HTMLURL)
			}
		}
	}
	p.kickDiscovery()
	return nil
}

// onInstallEvent atiende webhooks installation* (alta/baja): la API es la
// fuente de verdad, aquí solo se dispara discovery inmediato.
func (p *Program) onInstallEvent(id int64, account, action string) {
	p.log.Info("hook: evento instalación", "action", action, "account", account)
	p.kickDiscovery()
}

// kickDiscovery pide pasada inmediata (no bloquea).
func (p *Program) kickDiscovery() {
	select {
	case p.discoverKick <- struct{}{}:
	default:
	}
}

// onManifest guarda credenciales del manifest (PEM + settings) y devuelve
// la URL de instalación para el browser.
func (p *Program) onManifest(c *github.ManifestCredentials) (string, error) {
	if p.db == nil {
		return "", fmt.Errorf("sin DB")
	}
	keyPath := logx.AppKeyPath(p.cfgPath)
	if err := os.WriteFile(keyPath, []byte(c.PEM), 0o600); err != nil {
		return "", fmt.Errorf("guardar key: %w", err)
	}
	for k, v := range map[string]string{
		"github_app_id":         strconv.FormatInt(c.ID, 10),
		"github_key_file":       keyPath,
		"github_webhook_secret": c.WebhookSecret,
	} {
		if err := p.db.SetSetting(k, v); err != nil {
			return "", err
		}
	}
	slug := c.Slug
	if slug == "" {
		slug = "rkt-deployer"
	}
	_ = p.db.SetSetting("github_app_slug", slug)
	p.ghStale.Store(false)
	if gh, err := github.New(c.ID, keyPath, ""); err != nil {
		p.log.Warn("github app creada pero manager inválido", "err", err)
	} else {
		p.ghm.Store(gh)
		if p.autoStarted.CompareAndSwap(false, true) {
			go p.autoDeploy(p.live)
		}
	}
	p.log.Info("app github creada, instala en browser", "slug", slug)
	return "https://github.com/apps/" + slug + "/installations/new", nil
}

// dashboardURL arma el destino de vuelta tras instalar la App. El panel
// embebido vive en el listener de control (loopback), no en el proxy.
func dashboardURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return ""
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/settings?github=ok"
}

// setupPlan origen público + veredicto de entorno para el manifest.
// Lógica pura en github.PlanSetup (hogar único; el MCP la reutiliza).
func (p *Program) setupPlan() github.Plan {
	cfg := p.live.Get()
	return github.PlanSetup(p.db.GetSetting(store.KeyBaseDomain), p.db.GetSetting(store.KeyPublicIP),
		cfg.ListenHTTP, cfg.ListenHTTPS, nil)
}

// githubStatus estado de la GitHub App para dashboard/API.
func (p *Program) githubStatus() (map[string]any, error) {
	out := map[string]any{"configured": false}
	plan := p.setupPlan()
	origin, domain, local, tls := plan.Origin, plan.Domain, plan.Local, plan.TLS
	out["env"] = plan.Env.Kind
	if plan.Env.Blocked() {
		// Fail-closed: crear la App con este origen falla en GitHub o deja
		// el webhook sin entregar. Sin setup_url la UI no ofrece conectar.
		out["error"] = plan.Env.Problem
		out["fix"] = plan.Env.Fix
		out["local"] = local
		return out, nil
	}
	label := "local"
	if !local {
		label = strings.ReplaceAll(strings.Split(domain, ".")[0], "_", "-")
	}
	name := "rkt-" + label + "-" + strconv.FormatInt(time.Now().UnixNano()%10000, 10)
	setupURL := origin + "/_rkt/new-app?name=" + url.QueryEscape(name) +
		"&domain=" + url.QueryEscape(domain) + "&local=" + boolStr(local) + "&tls=" + boolStr(tls)
	// TTL generoso: el usuario se entretiene creando la App en GitHub y
	// el state one-time caducado obligaba a reiniciar el flujo.
	if p.db != nil {
		if state, err := p.db.NewSetupState(30 * time.Minute); err == nil {
			setupURL += "&state=" + url.QueryEscape(state)
		} else {
			p.log.Warn("setup state no generado", "err", err)
		}
	}
	out["setup_url"] = setupURL
	out["local"] = local
	cfg := p.live.Get()
	if cfg.Github == nil || p.gh() == nil || p.db.GetSetting("github_app_id") == "" {
		if p.ghStale.Load() {
			out["stale"] = true
		}
		if !local && (cfg.ListenHTTPS == "" || domain == "") {
			out["warning"] = "Para auto-deploy con webhooks en producción, configura base_domain y escucha HTTPS."
		}
		return out, nil
	}
	out["configured"] = true
	out["app_id"] = cfg.Github.AppID
	out["app_slug"] = cfg.Github.AppSlug
	out["account"] = p.db.GetSetting("github_account")
	out["repository_selection"] = p.db.GetSetting("github_repo_selection")
	out["install_url"] = p.db.GetSetting("github_install_url")
	if out["install_url"] == "" && cfg.Github.AppSlug != "" {
		out["install_url"] = "https://github.com/apps/" + cfg.Github.AppSlug + "/installations/new"
	}
	out["repositories"] = p.db.GetSetting("github_repo_count")
	out["webhook_active"] = (cfg.Github.WebhookSecret != "" && !local)
	if !local && (cfg.ListenHTTPS == "" || domain == "") {
		out["warning"] = "Para auto-deploy con webhooks en producción, configura base_domain y escucha HTTPS."
	}
	if p.gh() == nil {
		return out, nil
	}
	insts, err := p.gh().Installations()
	if err != nil {
		if errors.Is(err, github.ErrAppGone) {
			p.dropGithub()
			for _, k := range []string{
				"app_id", "app_slug", "account", "repository_selection",
				"install_url", "install_target", "repositories", "installations", "error",
			} {
				delete(out, k)
			}
			out["configured"] = false
			out["stale"] = true
			return out, nil
		}
		out["error"] = err.Error()
		return out, nil
	}
	out["installations"] = len(insts)
	if len(insts) == 0 {
		// La App existe pero aún no se ha instalado en ninguna cuenta/org.
		for _, k := range []string{
			"github_install_id", "github_account", "github_repo_selection",
			"github_install_url", "github_install_target", "github_repo_count",
		} {
			_ = p.db.SetSetting(k, "")
		}
		out["account"] = ""
		out["repositories"] = ""
		out["repository_selection"] = ""
		out["install_pending"] = true
		return out, nil
	}
	inst := insts[0]
	id := inst.ID
	if want := p.db.GetSetting("github_install_id"); want != "" {
		for _, i := range insts {
			if strconv.FormatInt(i.ID, 10) == want {
				id = i.ID
				break
			}
		}
	}
	out["account"] = inst.Account.Login
	out["repository_selection"] = inst.RepositorySelection
	out["install_url"] = inst.HTMLURL
	out["install_target"] = inst.TargetType
	if tok, err := p.gh().TokenFor(id); err == nil {
		if repos, err := p.gh().Repositories(tok); err == nil {
			out["repositories"] = strconv.Itoa(len(repos))
		}
	}
	return out, nil
}

// disconnectGithub desvincula limpiamente la GitHub App y borra su clave.
func (p *Program) disconnectGithub() error {
	p.dropGithub()
	p.ghStale.Store(false)
	p.log.Info("github: app desvinculada por petición del usuario")
	return nil
}

// testGithub prueba en vivo la autenticación contra la API de GitHub.
func (p *Program) testGithub() (map[string]any, error) {
	gh := p.gh()
	if gh == nil || p.db.GetSetting("github_app_id") == "" {
		return nil, fmt.Errorf("GitHub no está configurado")
	}
	start := time.Now()
	insts, err := gh.Installations()
	latency := time.Since(start).Milliseconds()
	if err != nil {
		if errors.Is(err, github.ErrAppGone) {
			p.dropGithub()
			return nil, fmt.Errorf("la GitHub App ya no existe o la clave privada no es válida")
		}
		return nil, fmt.Errorf("error al conectar con GitHub (%dms): %w", latency, err)
	}
	repoCount := 0
	if len(insts) > 0 {
		if tok, err := gh.TokenFor(insts[0].ID); err == nil {
			if repos, err := gh.Repositories(tok); err == nil {
				repoCount = len(repos)
			}
		}
	}
	account := p.db.GetSetting("github_account")
	if account == "" && len(insts) > 0 {
		account = insts[0].Account.Login
	}
	return map[string]any{
		"ok":                   true,
		"account":              account,
		"installations":        len(insts),
		"repositories":         repoCount,
		"repository_selection": p.db.GetSetting("github_repo_selection"),
		"latency_ms":           latency,
	}, nil
}

// dropGithub limpia la configuración local cuando la App ya no existe
// en GitHub (borrada) o su clave dejó de autenticar. Permite recrearla
// desde Ajustes sin reiniciar ni dejar estado fantasma.
func (p *Program) dropGithub() {
	for _, k := range []string{
		"github_app_id", "github_app_slug", "github_key_file", "github_webhook_secret",
		"github_install_id", "github_account", "github_repo_selection",
		"github_install_url", "github_install_target", "github_repo_count",
	} {
		_ = p.db.SetSetting(k, "")
	}
	_ = os.Remove(logx.AppKeyPath(p.cfgPath))
	p.ghm.Store(nil)
	p.ghStale.Store(true)
	p.log.Warn("github: la App ya no existe en GitHub; configuración local limpiada")
}

// githubRepos lista repos de todas las instalaciones (personales y orgs).
func (p *Program) githubRepos() ([]github.Repo, error) {
	gh := p.gh()
	if gh == nil {
		return nil, fmt.Errorf("sin github app")
	}
	insts, err := gh.Installations()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []github.Repo{}
	for _, inst := range insts {
		tok, err := gh.TokenFor(inst.ID)
		if err != nil {
			continue
		}
		repos, err := gh.Repositories(tok)
		if err != nil {
			continue
		}
		for _, r := range repos {
			if seen[r.FullName] {
				continue
			}
			seen[r.FullName] = true
			out = append(out, github.Repo{
				FullName: r.FullName, DefaultBranch: r.DefaultBranch,
				Private: r.Private, Owner: r.Owner.Login,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	return out, nil
}

// githubRevalidate relee las instalaciones de la App y detecta cuentas/orgs
// donde falta instalarla (con el PAT del usuario si hay).
func (p *Program) githubRevalidate() (github.Report, error) {
	gh := p.gh()
	if gh == nil {
		return github.Report{}, fmt.Errorf("GitHub App no configurada: créala desde Ajustes → GitHub")
	}
	slug := p.db.GetSetting("github_app_slug")
	if slug == "" {
		if cfg := p.live.Get(); cfg.Github != nil {
			slug = cfg.Github.AppSlug
		}
	}
	rep, err := gitcred.Revalidate(p.db, logx.SecretsDir(p.cfgPath), gh, slug)
	if err != nil {
		return rep, err
	}
	n := 0
	for _, in := range rep.Installations {
		n += in.Repos
	}
	_ = p.db.SetSetting("github_repo_count", strconv.Itoa(n))
	return rep, nil
}
