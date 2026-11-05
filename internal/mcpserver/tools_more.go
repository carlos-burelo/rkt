// Tools extendidas del MCP: control total (rutas, DNS, flota, settings,
// usuarios, observabilidad, backup y poda). Mismo backend que CLI/API.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt-proxy/internal/alerts"
	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/hosts"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/weblog"
)

type nameArg struct {
	App string `json:"app" jsonschema:"nombre de la app"`
}
type routesAddArg struct {
	Hosts  []string `json:"hosts" jsonschema:"hosts a enrutar"`
	Target string   `json:"target" jsonschema:"backend http(s)://127.0.0.1:puerto"`
}
type routesUpdateArg struct {
	Host   string `json:"host" jsonschema:"host existente en la ruta"`
	Target string `json:"target" jsonschema:"nuevo backend"`
}
type hostArg struct {
	Host string `json:"host" jsonschema:"host (ej app.localhost)"`
}
type verifyArg struct {
	Host string `json:"host" jsonschema:"host a verificar"`
}
type envDelArg struct {
	App string `json:"app" jsonschema:"nombre de la app"`
	Key string `json:"key" jsonschema:"CLAVE"`
}
type envGlobalArg struct {
	Key   string `json:"key" jsonschema:"CLAVE a definir"`
	Value string `json:"value" jsonschema:"valor (admite ${VAR})"`
}
type envGlobalListArg struct {
	IncludeSecrets bool `json:"include_secrets,omitempty" jsonschema:"incluir valores (default false)"`
}
type deployWaitArg struct {
	ID       int64  `json:"id,omitempty" jsonschema:"id del deployment (o usa app)"`
	App      string `json:"app,omitempty" jsonschema:"esperar el último deployment de la app"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 120, máx 600)"`
}
type logSearchArg struct {
	App     string `json:"app,omitempty" jsonschema:"app (vacío = operativo)"`
	Pattern string `json:"pattern" jsonschema:"regex a buscar"`
	N       int    `json:"n,omitempty" jsonschema:"máximo de líneas (default 50)"`
}
type settingsSetArg struct {
	Key   string `json:"key" jsonschema:"clave de configuración"`
	Value string `json:"value" jsonschema:"valor"`
}
type alertAddArg struct {
	App       string  `json:"app,omitempty" jsonschema:"app o *"`
	Kind      string  `json:"kind" jsonschema:"failed|error_rate"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"umbral (0.1 = 10% en error_rate)"`
	WindowMin int     `json:"window_min,omitempty" jsonschema:"ventana minutos (default 60)"`
}
type alertDelArg struct {
	ID int64 `json:"id" jsonschema:"id de la regla"`
}
type alertUpdateArg struct {
	ID        int64    `json:"id" jsonschema:"id de la regla"`
	App       string   `json:"app,omitempty" jsonschema:"app o *"`
	Kind      string   `json:"kind,omitempty" jsonschema:"failed|error_rate|latency_p95|memory_mb|down_min|disk_pct"`
	Threshold *float64 `json:"threshold,omitempty" jsonschema:"umbral"`
	WindowMin *int     `json:"window_min,omitempty" jsonschema:"ventana minutos"`
	Enabled   *bool    `json:"enabled,omitempty" jsonschema:"activar/desactivar"`
	ChannelID *int64   `json:"channel_id,omitempty" jsonschema:"canal (0 = sin canal)"`
}
type userAddArg struct {
	Name     string `json:"name" jsonschema:"usuario"`
	Password string `json:"password" jsonschema:"contraseña (mín 12)"`
	Role     string `json:"role,omitempty" jsonschema:"viewer|operator|admin (default viewer)"`
}
type userArg struct {
	Name string `json:"name" jsonschema:"usuario"`
}
type userPassArg struct {
	Name     string `json:"name" jsonschema:"usuario"`
	Password string `json:"password" jsonschema:"contraseña nueva (mín 12)"`
}
type exportArg struct {
	IncludeSecrets bool `json:"include_secrets,omitempty" jsonschema:"incluir valores de Env (default false)"`
}
type importArg struct {
	JSON    string `json:"json" jsonschema:"config JSON completa (export de config_export)"`
	Confirm string `json:"confirm" jsonschema:"escribe replace para confirmar"`
}
type pruneArg struct {
	DeploymentsDays int    `json:"deployments_days,omitempty" jsonschema:"borrar deployments terminados con más días (default 30)"`
	EventsDays      int    `json:"events_days,omitempty" jsonschema:"borrar eventos con más días (default 30)"`
	Confirm         string `json:"confirm" jsonschema:"escribe prune para confirmar"`
}

func jsonText(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// load abre config+DB y la cierra al terminar el handler.
func (s *Server) load() (*config.Config, *store.DB, error) {
	return cfgfile.LoadMerged(s.cfgPath)
}

func (s *Server) doAppGet(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	return jsonText(appview.App(cfg, *a))
}

func (s *Server) doAppEnvKeys(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	keys := make([]string, 0, len(a.Env))
	for k := range a.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return jsonText(map[string]any{"app": app, "keys": keys})
}

func (s *Server) doEnvDelete(a envDelArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		app := cfgfile.FindApp(cp, a.App)
		if app == nil {
			return fmt.Errorf("app %q inexistente", a.App)
		}
		if _, ok := app.Env[a.Key]; !ok {
			return fmt.Errorf("%s no tiene %s", a.App, a.Key)
		}
		return cfgfile.UnsetEnv(cp, a.App, a.Key)
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s borrada (aplica en ≤10s)", a.App, a.Key), nil
}

func (s *Server) doEnvGlobalList(a envGlobalListArg) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	keys := make([]string, 0, len(cfg.GlobalEnv))
	for k := range cfg.GlobalEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if a.IncludeSecrets {
		out := make(map[string]string, len(keys))
		for _, k := range keys {
			out[k] = cfg.GlobalEnv[k]
		}
		return jsonText(map[string]any{"keys": keys, "values": out, "count": len(keys)})
	}
	return jsonText(map[string]any{"keys": keys, "count": len(keys)})
}

func (s *Server) doEnvGlobalSet(a envGlobalArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := cfgfile.SetGlobalEnv(db, a.Key, a.Value); err != nil {
		return "", err
	}
	return fmt.Sprintf("global %s guardada (las apps la heredan al reiniciar)", a.Key), nil
}

func (s *Server) doEnvGlobalDelete(a envGlobalArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateInfra(db, func(cp *config.Config) error {
		if _, ok := cp.GlobalEnv[a.Key]; !ok {
			return fmt.Errorf("no existe la global %s", a.Key)
		}
		cfgfile.UnsetGlobalEnvValue(cp, a.Key)
		return nil
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("global %s eliminada", a.Key), nil
}

func (s *Server) doRoutesAdd(a routesAddArg) (string, error) {
	if len(a.Hosts) == 0 || a.Target == "" {
		return "", fmt.Errorf("faltan hosts/target")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		return cfgfile.AddRoute(cp, a.Hosts, a.Target)
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("ruta %s → %s creada", strings.Join(a.Hosts, ","), a.Target), nil
}

func (s *Server) doRoutesUpdate(a routesUpdateArg) (string, error) {
	if a.Host == "" || a.Target == "" {
		return "", fmt.Errorf("faltan host/target")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	found := false
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		ok, err := cfgfile.SetRouteTarget(cp, a.Host, a.Target)
		found = ok
		return err
	}); err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("host %q no existe", a.Host)
	}
	return fmt.Sprintf("ruta %s → %s", a.Host, a.Target), nil
}

func (s *Server) doRoutesRemove(a hostArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	removed := false
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		ok, err := cfgfile.RemoveRouteHost(cp, a.Host)
		removed = ok
		return err
	}); err != nil {
		return "", err
	}
	if !removed {
		return "", fmt.Errorf("host %q no existe", a.Host)
	}
	return fmt.Sprintf("host %s quitado", a.Host), nil
}

func (s *Server) doDomainsSuggest(name string) (string, error) {
	slug := strings.Trim(regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		return "", fmt.Errorf("falta name")
	}
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	used := map[string]bool{}
	for _, r := range cfg.Routes {
		for _, h := range r.Hosts {
			used[strings.ToLower(h)] = true
		}
	}
	out := []map[string]string{}
	add := func(h, kind, note string) {
		if h == "" || used[strings.ToLower(h)] {
			return
		}
		out = append(out, map[string]string{"host": h, "kind": kind, "note": note})
	}
	add(slug+".localhost", "local", "funciona sin editar hosts")
	add(slug+".test", "local", "requiere hosts → 127.0.0.1")
	if base := strings.Trim(strings.TrimSpace(db.GetSetting(store.KeyBaseDomain)), "."); base != "" {
		add(slug+"."+base, "base", "tu dominio base")
	}
	return jsonText(map[string]any{"name": slug, "suggestions": out})
}

func (s *Server) doVerifyDomain(host string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if host == "" {
		return "", fmt.Errorf("falta host")
	}
	out := map[string]any{"host": host, "dns_ok": appview.ResolvesLocal(host)}
	if !cfg.Healthz.Enabled {
		out["error"] = "healthz desactivado"
		return jsonText(out)
	}
	// request al proxy HTTP por su listen local con Host: host
	addr := cfg.ListenHTTP
	if addr == "" {
		out["error"] = "listen_http vacío (modo solo TLS): usa el navegador"
		return jsonText(out)
	}
	addr = strings.TrimPrefix(addr, "0.0.0.0")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 4 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
	req.Host = host
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		out["http_error"] = err.Error()
		return jsonText(out)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	out["status"] = resp.StatusCode
	out["ms"] = time.Since(start).Milliseconds()
	out["body"] = string(body)
	return jsonText(out)
}

func (s *Server) doHostsList() (string, error) {
	list, err := hosts.List()
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"path": hosts.Path(), "hosts": list})
}

func (s *Server) doHostsAdd(host string) (string, error) {
	if err := hosts.Add(host); err != nil {
		return "", fmt.Errorf("%v (requiere administrador)", err)
	}
	return fmt.Sprintf("%s → 127.0.0.1 en %s", host, hosts.Path()), nil
}

func (s *Server) doHostsRemove(host string) (string, error) {
	if err := hosts.Remove(host); err != nil {
		return "", fmt.Errorf("%v (requiere administrador)", err)
	}
	return fmt.Sprintf("%s quitado de hosts", host), nil
}

func (s *Server) doDeploymentGet(id int64) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	deps, err := db.ListDeployments("", 2000)
	if err != nil {
		return "", err
	}
	for _, d := range deps {
		if d.ID == id {
			return jsonText(d)
		}
	}
	return "", fmt.Errorf("deployment %d inexistente", id)
}

func (s *Server) doDeployWait(a deployWaitArg) (string, error) {
	timeout := a.TimeoutS
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for {
		deps, err := db.ListDeployments(a.App, 50)
		if err != nil {
			return "", err
		}
		for _, d := range deps {
			if a.ID > 0 && d.ID != a.ID {
				continue
			}
			if a.ID == 0 && a.App != "" && d.App != a.App {
				continue
			}
			if d.Status != "running" {
				return jsonText(d)
			}
			break
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("deployment sin terminar tras %ds", timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Server) doGitStatus(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	run := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", a.Dir}, args...)...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	dirty := run("status", "--porcelain")
	return jsonText(map[string]any{
		"app": app, "dir": a.Dir,
		"head":        run("rev-parse", "--short", "HEAD"),
		"branch":      run("branch", "--show-current"),
		"remote":      run("remote", "get-url", "origin"),
		"dirty_files": len(strings.Split(dirty, "\n")) - boolToInt(dirty == ""),
		"dirty":       dirty != "",
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) doDiffRev(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	prev := a.Dir + ".prev"
	if _, err := os.Stat(prev); err != nil {
		return "", fmt.Errorf("sin backup previo para %s", app)
	}
	prevRev := ""
	if out, err := exec.Command("git", "-C", prev, "rev-parse", "HEAD").Output(); err == nil {
		prevRev = strings.TrimSpace(string(out))
	}
	curRev := ""
	if out, err := exec.Command("git", "-C", a.Dir, "rev-parse", "HEAD").Output(); err == nil {
		curRev = strings.TrimSpace(string(out))
	}
	changed := ""
	if prevRev != "" && curRev != "" {
		if out, err := exec.Command("git", "-C", a.Dir, "diff", "--name-status", prevRev, curRev).Output(); err == nil {
			changed = string(out)
		}
	}
	return jsonText(map[string]any{"app": app, "prev": prevRev, "current": curRev, "changed": changed})
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *Server) doDiskUsage(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	out := map[string]any{"app": app, "dir": a.Dir}
	for _, suffix := range []string{"", ".prev", ".staging"} {
		p := a.Dir + suffix
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out[suffixOrLive(suffix)] = dirSize(p)
		}
	}
	return jsonText(out)
}

func suffixOrLive(s string) string {
	if s == "" {
		return "live"
	}
	return strings.TrimPrefix(s, ".")
}

func (s *Server) doLogSearch(a logSearchArg) (string, error) {
	path := logx.Path(s.cfgPath)
	if a.App != "" {
		path = filepath.Join(logx.DirFor(s.cfgPath), "apps", a.App+".log")
	}
	txt, err := logx.Tail(path, 5000)
	if err != nil {
		return "", err
	}
	rx, err := regexp.Compile(a.Pattern)
	if err != nil {
		return "", fmt.Errorf("regex inválida: %w", err)
	}
	limit := a.N
	if limit <= 0 {
		limit = 50
	}
	matches := []string{}
	for _, line := range strings.Split(txt, "\n") {
		if rx.MatchString(line) {
			matches = append(matches, line)
		}
	}
	if len(matches) > limit {
		matches = matches[len(matches)-limit:]
	}
	return strings.Join(matches, "\n"), nil
}

func (s *Server) doUsageSummary() (string, error) {
	entries := weblog.ParseFile(logx.AccessPath(s.cfgPath))
	return jsonText(weblog.Summarize(entries))
}

func (s *Server) doProxyMap() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	out := []map[string]any{}
	for _, r := range cfg.Routes {
		rs := map[string]any{"hosts": r.Hosts, "target": r.Target}
		if u, err := url.Parse(r.Target); err == nil && u.Host != "" {
			rs["ok"] = health.BackendOK(u.Host)
		}
		dns := []bool{}
		for _, h := range r.Hosts {
			dns = append(dns, appview.ResolvesLocal(h))
		}
		rs["dns_ok"] = dns
		out = append(out, rs)
	}
	return jsonText(out)
}

func (s *Server) doBackendsProbe() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	seen := map[string]bool{}
	out := []map[string]any{}
	for _, r := range cfg.Routes {
		u, err := url.Parse(r.Target)
		if err != nil || u.Host == "" || seen[u.Host] {
			continue
		}
		seen[u.Host] = true
		out = append(out, map[string]any{"target": r.Target, "ok": health.BackendOK(u.Host)})
	}
	return jsonText(out)
}

func (s *Server) doSettingsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	out := map[string]string{}
	for _, k := range store.KnownSettings() {
		out[k] = db.GetSetting(k)
	}
	return jsonText(out)
}

func (s *Server) doSettingsSet(a settingsSetArg) (string, error) {
	known := false
	for _, k := range store.KnownSettings() {
		if k == a.Key {
			known = true
		}
	}
	if !known {
		return "", fmt.Errorf("setting desconocido: %s", a.Key)
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.SetSetting(a.Key, a.Value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s actualizado", a.Key), nil
}

func (s *Server) doAlertsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rules, err := db.ListRules()
	if err != nil {
		return "", err
	}
	return jsonText(rules)
}

func (s *Server) doAlertsAdd(a alertAddArg) (string, error) {
	app := a.App
	if app == "" {
		app = "*"
	}
	kind := a.Kind
	if kind == "" {
		kind = "failed"
	}
	th := a.Threshold
	if th == 0 {
		th = 1
	}
	win := a.WindowMin
	if win <= 0 {
		win = 60
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	id, err := db.AddRule(app, kind, th, win)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("regla #%d creada (%s %s)", id, app, kind), nil
}

func (s *Server) doAlertsRemove(id int64) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.DeleteRule(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("regla #%d borrada", id), nil
}

func (s *Server) doAlertsUpdate(a alertUpdateArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rules, err := db.ListRules()
	if err != nil {
		return "", err
	}
	var rule *store.AlertRule
	for i := range rules {
		if rules[i].ID == a.ID {
			rule = &rules[i]
		}
	}
	if rule == nil {
		return "", fmt.Errorf("regla #%d inexistente", a.ID)
	}
	app, kind := rule.App, rule.Kind
	th, win := rule.Threshold, rule.WindowMin
	enabled, channel := rule.Enabled, rule.ChannelID
	if a.App != "" {
		app = a.App
	}
	if a.Kind != "" {
		kind = a.Kind
	}
	if a.Threshold != nil {
		th = *a.Threshold
	}
	if a.WindowMin != nil {
		win = *a.WindowMin
	}
	if a.Enabled != nil {
		enabled = *a.Enabled
	}
	if a.ChannelID != nil {
		channel = *a.ChannelID
	}
	if err := db.UpdateRule(a.ID, app, kind, th, win, enabled, channel); err != nil {
		return "", err
	}
	return fmt.Sprintf("regla #%d actualizada", a.ID), nil
}

func (s *Server) doAlertsTest(id int64) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rules, err := db.ListRules()
	if err != nil {
		return "", err
	}
	for _, rule := range rules {
		if rule.ID == id {
			return alerts.Describe(rule, alerts.Metrics{}), nil
		}
	}
	return "", fmt.Errorf("regla #%d inexistente", id)
}

func (s *Server) doAlertChannelsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	chans, err := db.ListChannels()
	if err != nil {
		return "", err
	}
	return jsonText(chans)
}

func (s *Server) doDiscoverStatus() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	out := map[string]any{"configured": cfg.Github != nil}
	if cfg.Github != nil {
		keyOK := false
		if _, err := os.Stat(cfg.Github.KeyFile); err == nil {
			keyOK = true
		}
		out["app_slug"] = cfg.Github.AppSlug
		out["poll_interval"] = cfg.Github.PollInterval
		out["key_file_ok"] = keyOK
		out["webhook_secret"] = cfg.Github.WebhookSecret != ""
		out["account"] = db.GetSetting("github_account")
		out["repository_selection"] = db.GetSetting("github_repo_selection")
		out["repositories"] = db.GetSetting("github_repo_count")
		out["install_url"] = db.GetSetting("github_install_url")
		out["all_repositories"] = db.GetSetting("github_repo_selection") == "all"
	}
	out["base_domain"] = db.GetSetting(store.KeyBaseDomain)
	return jsonText(out)
}

func validRole(r string) bool {
	return r == store.RoleViewer || r == store.RoleOperator || r == store.RoleAdmin
}

func (s *Server) doUsersList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	users, err := db.ListUsers()
	if err != nil {
		return "", err
	}
	return jsonText(users)
}

func (s *Server) doUsersAdd(a userAddArg) (string, error) {
	if a.Name == "" || len(a.Password) < 12 {
		return "", fmt.Errorf("usuario vacío o contraseña <12")
	}
	role := a.Role
	if role == "" {
		role = store.RoleViewer
	}
	if !validRole(role) {
		return "", fmt.Errorf("rol inválido: %s", role)
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.CreateUser(a.Name, a.Password, role); err != nil {
		return "", err
	}
	return fmt.Sprintf("usuario %s (%s) creado", a.Name, role), nil
}

func (s *Server) doUsersRemove(a userArg) (string, error) {
	if a.Name == "" {
		return "", fmt.Errorf("falta name")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.DeleteUser(a.Name); err != nil {
		return "", err
	}
	return fmt.Sprintf("usuario %s borrado", a.Name), nil
}

func (s *Server) doUsersPasswd(a userPassArg) (string, error) {
	if a.Name == "" || len(a.Password) < 12 {
		return "", fmt.Errorf("usuario vacío o contraseña <12")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.SetPassword(a.Name, a.Password); err != nil {
		return "", err
	}
	return fmt.Sprintf("password de %s actualizado (sesiones invalidadas)", a.Name), nil
}

func (s *Server) doConfigExport(a exportArg) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	cp := *cfg
	if !a.IncludeSecrets {
		cp.Apps = make([]config.App, len(cfg.Apps))
		for i, x := range cfg.Apps {
			x.Env = nil
			cp.Apps[i] = x
		}
	}
	settings := map[string]string{}
	for _, k := range store.KnownSettings() {
		if v := db.GetSetting(k); v != "" {
			settings[k] = v
		}
	}
	out, err := config.ExportJSON(&cp, settings)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *Server) doConfigImport(a importArg) (string, error) {
	if a.Confirm != "replace" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"replace"}`)
	}
	cfg, settings, err := config.ImportJSON([]byte(a.JSON))
	if err != nil {
		return "", err
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	// Los exports saneados no traen Env: conserva el actual por app.
	prev, err := db.LoadFleet()
	if err != nil {
		return "", err
	}
	oldEnv := map[string]map[string]string{}
	for _, a := range prev.Apps {
		oldEnv[a.Name] = a.Env
	}
	for i := range cfg.Apps {
		if len(cfg.Apps[i].Env) == 0 {
			if env, ok := oldEnv[cfg.Apps[i].Name]; ok {
				cfg.Apps[i].Env = env
			}
		}
	}
	if err := cfgfile.ReplaceAll(db, cfg, settings); err != nil {
		return "", err
	}
	return fmt.Sprintf("config reemplazada: %d apps, %d rutas", len(cfg.Apps), len(cfg.Routes)), nil
}

func (s *Server) doPrune(a pruneArg) (string, error) {
	if a.Confirm != "prune" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"prune"}`)
	}
	depDays, evDays := a.DeploymentsDays, a.EventsDays
	if depDays <= 0 {
		depDays = 30
	}
	if evDays <= 0 {
		evDays = 30
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	deps, err := db.PruneDeployments(time.Now().AddDate(0, 0, -depDays))
	if err != nil {
		return "", err
	}
	evs, err := db.PruneEvents(time.Now().AddDate(0, 0, -evDays))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("podado: %d deployments, %d eventos", deps, evs), nil
}

// registerMore añade el catálogo extendido al servidor MCP.
func (s *Server) registerMore(srv *mcp.Server) {
	add := func(name, desc string, fn func() (string, error)) {
		mcp.AddTool(srv, &mcp.Tool{Name: name, Description: desc},
			func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
				return text(fn())
			})
	}
	// tool con argumento app
	mcp.AddTool(srv, &mcp.Tool{Name: "app_get", Description: "Detalle de una app (dominio, puerto, estático, deploy, dns_ok)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppGet(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "app_env_keys", Description: "Nombres de variables de entorno (nunca valores)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppEnvKeys(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_delete", Description: "Borra una variable de entorno de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvDelete(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_global_list", Description: "Variables globales de todas las apps (claves; valores solo con include_secrets)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envGlobalListArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvGlobalList(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_global_set", Description: "Crea/actualiza una variable global; todas las apps la heredan al reiniciar."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envGlobalArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvGlobalSet(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "env_global_delete", Description: "Elimina una variable global de todas las apps."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envGlobalArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvGlobalDelete(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "routes_add", Description: "Crea una ruta hosts → target."},
		func(ctx context.Context, req *mcp.CallToolRequest, a routesAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesAdd(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "routes_update", Description: "Cambia el target de la ruta que contiene host."},
		func(ctx context.Context, req *mcp.CallToolRequest, a routesUpdateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesUpdate(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "routes_remove", Description: "Quita un host de sus rutas."},
		func(ctx context.Context, req *mcp.CallToolRequest, a hostArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesRemove(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "domains_suggest", Description: "Dominios libres para un nombre (localhost/test/base)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDomainsSuggest(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "verify_domain", Description: "Verifica DNS local y respuesta HTTP del proxy para un host."},
		func(ctx context.Context, req *mcp.CallToolRequest, a verifyArg) (*mcp.CallToolResult, any, error) {
			return text(s.doVerifyDomain(a.Host))
		})
	add("hosts_list", "Hosts gestionados por rkt en el archivo hosts.", s.doHostsList)
	mcp.AddTool(srv, &mcp.Tool{Name: "hosts_add", Description: "Añade host → 127.0.0.1 al archivo hosts (admin)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a hostArg) (*mcp.CallToolResult, any, error) {
			return text(s.doHostsAdd(a.Host))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "hosts_remove", Description: "Quita un host del bloque de rkt en hosts (admin)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a hostArg) (*mcp.CallToolResult, any, error) {
			return text(s.doHostsRemove(a.Host))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "deployment_get", Description: "Estado de un deployment por id."},
		func(ctx context.Context, req *mcp.CallToolRequest, a deployWaitArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeploymentGet(a.ID))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "deploy_wait", Description: "Espera a que un deployment termine (running → ok/failed)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a deployWaitArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeployWait(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "git_status", Description: "HEAD, rama, remote y suciedad del dir live de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doGitStatus(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "diff_rev", Description: "Archivos cambiados entre live.prev y live."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDiffRev(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "disk_usage", Description: "Bytes de live/prev/staging de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDiskUsage(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "log_search", Description: "Busca regex en logs (app u operativo), últimas coincidencias."},
		func(ctx context.Context, req *mcp.CallToolRequest, a logSearchArg) (*mcp.CallToolResult, any, error) {
			return text(s.doLogSearch(a))
		})
	add("usage_summary", "Resumen de tráfico desde access.log (total, bytes, clases, días, top).", s.doUsageSummary)
	add("proxy_map", "Rutas con backend ok y dns_ok (vista /proxy).", s.doProxyMap)
	add("backends_probe", "Dial TCP a cada backend único de las rutas.", s.doBackendsProbe)
	add("settings_list", "Claves de configuración y sus valores.", s.doSettingsList)
	mcp.AddTool(srv, &mcp.Tool{Name: "settings_set", Description: "Cambia una clave de configuración (base_domain, node_bin, quotas…)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a settingsSetArg) (*mcp.CallToolResult, any, error) {
			return text(s.doSettingsSet(a))
		})
	add("alerts_list", "Reglas de alerta configuradas.", s.doAlertsList)
	mcp.AddTool(srv, &mcp.Tool{Name: "alerts_add", Description: "Crea regla de alerta (failed|error_rate)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsAdd(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "alerts_remove", Description: "Borra una regla de alerta por id."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsRemove(a.ID))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "alerts_update", Description: "Edita una regla de alerta (app, kind, umbral, ventana, enabled, canal)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertUpdateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsUpdate(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "alerts_test", Description: "Describe el disparo de una regla sin ejecutarlo."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsTest(a.ID))
		})
	add("alert_channels_list", "Canales webhook configurados.", s.doAlertChannelsList)
	add("discover_status", "Estado de GitHub App/base_domain para auto-registro.", s.doDiscoverStatus)
	add("users_list", "Usuarios (sin hashes) y roles.", s.doUsersList)
	mcp.AddTool(srv, &mcp.Tool{Name: "users_add", Description: "Crea usuario local (password ≥12). Queda en el historial del agente."},
		func(ctx context.Context, req *mcp.CallToolRequest, a userAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doUsersAdd(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "users_remove", Description: "Borra un usuario local."},
		func(ctx context.Context, req *mcp.CallToolRequest, a userArg) (*mcp.CallToolResult, any, error) {
			return text(s.doUsersRemove(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "users_passwd", Description: "Cambia password de un usuario (password ≥12)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a userPassArg) (*mcp.CallToolResult, any, error) {
			return text(s.doUsersPasswd(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "config_export", Description: "Exporta configuración y apps en JSON (secretos solo con include_secrets)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a exportArg) (*mcp.CallToolResult, any, error) {
			return text(s.doConfigExport(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "config_import", Description: "Reemplaza configuración y apps desde JSON (confirm=replace). Destructivo."},
		func(ctx context.Context, req *mcp.CallToolRequest, a importArg) (*mcp.CallToolResult, any, error) {
			return text(s.doConfigImport(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "prune", Description: "Poda deployments/events viejos (confirm=prune)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pruneArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPrune(a))
		})
	s.registerMonitor(srv)
}
