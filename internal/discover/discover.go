// Package discover: auto-registro de apps desde repos instalados.
// Repo nuevo + preset conocido = app + route + deploy inmediato.
// Sin preset (stack raro) se avisa con el comando manual.
package discover

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/store"
)

// presetBinKey qué setting de binario usa cada preset.
var presetBinKey = map[string]string{
	"next": store.KeyNodeBin, "node": store.KeyNodeBin, "python": store.KeyPythonBin,
}

// PresetBinKey expone la clave de setting del binario de un preset.
func PresetBinKey(preset string) (string, bool) {
	k, ok := presetBinKey[preset]
	return k, ok
}

// Deps dependencias inyectables (tests usan fakes).
type Deps struct {
	GH       *github.Manager
	DB       *store.DB
	Log      *slog.Logger
	DeployFn func(name, actor string) (string, error)
	// DirBase dir padre de apps (default C:\apps). Tests lo aíslan.
	DirBase string
}

// Run registra repos no conocidos de TODAS las instalaciones (vía API,
// fuente de verdad) y los despliega. Idempotente. Devuelve registrados.
func Run(d Deps) ([]string, error) {
	installs, err := d.GH.Installations()
	if err != nil {
		return nil, fmt.Errorf("instalaciones: %w", err)
	}
	registered := []string{}
	for _, inst := range installs {
		tok, err := d.GH.TokenFor(inst.ID)
		if err != nil {
			d.Log.Warn("discovery: sin token", "installation", inst.ID, "err", err)
			continue
		}
		repos, err := d.GH.Repositories(tok)
		if err != nil {
			d.Log.Warn("discovery: sin repos", "installation", inst.ID, "err", err)
			continue
		}
		known, err := knownRepos(d.DB)
		if err != nil {
			return nil, err
		}
		for _, r := range repos {
			owner, name, ok := splitFull(r.FullName)
			if !ok {
				continue
			}
			if known[strings.ToLower(r.FullName)] {
				continue
			}
			if err := register(d, owner, name, r.DefaultBranch, tok); err != nil {
				d.Log.Warn("auto-registro fallido", "repo", r.FullName, "err", err)
				continue
			}
			known[strings.ToLower(r.FullName)] = true
			registered = append(registered, name)
		}
	}
	return registered, nil
}

func knownRepos(db *store.DB) (map[string]bool, error) {
	fleet, err := db.LoadFleet()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for i := range fleet.Apps {
		if fleet.Apps[i].Deploy != nil {
			if o, r, ok := github.ParseRepo(fleet.Apps[i].Deploy.Repo); ok {
				out[strings.ToLower(o+"/"+r)] = true
			}
		}
	}
	return out, nil
}

func splitFull(full string) (string, string, bool) {
	parts := strings.Split(full, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func register(d Deps, owner, repo, branch, tok string) error {
	plan, err := detectPlanRemote(d.GH, owner, repo)
	if err != nil {
		return err
	}
	if plan.Kind == "" {
		d.Log.Warn("repo sin plan de build conocido, regístralo manual",
			"repo", owner+"/"+repo, "ayuda", "apps add")
		return nil
	}
	base := d.DB.GetSetting(store.KeyBaseDomain)
	if base == "" {
		return fmt.Errorf("sin base_domain (settings set base_domain=...)")
	}
	// plan estático no necesita binario; proceso sí.
	var bin string
	if !plan.IsStatic() {
		binKey, ok := presetBinKey[plan.Kind]
		if !ok {
			return fmt.Errorf("plan %q sin binario", plan.Kind)
		}
		bin = d.DB.GetSetting(binKey)
		if bin == "" {
			return fmt.Errorf("sin %s (settings set %s=...)", binKey, binKey)
		}
		if st, err := os.Stat(bin); err != nil || st.IsDir() {
			return fmt.Errorf("%s no es ejecutable: %s", binKey, bin)
		}
	}
	port, err := freePort(d.DB)
	if err != nil {
		return err
	}
	name := slug(repo)
	if name == "" {
		return fmt.Errorf("repo %q sin nombre válido", repo)
	}
	if appExists(d.DB, name) {
		return fmt.Errorf("app %q ya existe (registro manual?)", name)
	}
	domain := uniqueDomain(d.DB, name, owner, base)
	dir := d.DirBase
	if dir == "" {
		dir = `C:\apps`
	}
	dir = filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("crear dir: %w", err)
	}
	app := config.App{
		Name: name, Env: map[string]string{"PORT": strconv.Itoa(port)},
		Deploy: &config.Deploy{
			Repo: "https://github.com/" + owner + "/" + repo + ".git",
			Ref:  branch, Auto: true,
			Build: append(append([]string{}, plan.Install...), plan.Build...),
		},
	}
	app.SetDelays(5*time.Second, 10*time.Second)
	app.Dir = dir
	if plan.IsStatic() {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("binario propio: %w", err)
		}
		app.Command = exe
		app.Args = []string{"__static", plan.Static(dir)}
		app.Static = plan.Static(dir)
	} else {
		app.Command = bin
		if len(plan.Start) > 1 {
			app.Args = plan.Start[1:]
		}
	}
	if err := d.DB.UpsertApp(&app); err != nil {
		return err
	}
	if _, err := d.DB.AddRoute([]string{domain}, fmt.Sprintf("http://127.0.0.1:%d", port)); err != nil {
		return err
	}
	d.Log.Info("app auto-registrada", "app", name, "domain", domain, "port", port)
	_ = d.DB.InsertEvent("discover", name, domain)
	if d.DeployFn != nil {
		if rev, err := d.DeployFn(name, "discover"); err != nil {
			d.Log.Error("auto-deploy inicial fallido", "app", name, "err", err)
		} else {
			d.Log.Info("auto-deploy inicial ok", "app", name, "rev", rev)
		}
	}
	return nil
}

// detectPlanRemote clasifica un repo por package.json (público o token).
func detectPlanRemote(gh *github.Manager, owner, repo string) (buildpack.Plan, error) {
	branch := "main"
	content, ok, err := gh.FileContent(owner, repo, "package.json", branch, "")
	if err != nil {
		return buildpack.Plan{}, err
	}
	if ok {
		_, hasReq, _ := gh.FileContent(owner, repo, "requirements.txt", branch, "")
		return buildpack.FromPackageJSON(content, hasReq), nil
	}
	if _, hasReq, err := gh.FileContent(owner, repo, "requirements.txt", branch, ""); err == nil && hasReq {
		return buildpack.FromPackageJSON("", true), nil
	}
	return buildpack.Plan{}, nil
}

// slug minúsculas, [^a-z0-9] -> -.
func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteByte(c)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// uniqueDomain evita colisión de dominio (dos owners, mismo repo).
func appExists(db *store.DB, name string) bool {
	fleet, err := db.LoadFleet()
	if err != nil {
		return false
	}
	for i := range fleet.Apps {
		if fleet.Apps[i].Name == name {
			return true
		}
	}
	return false
}

func uniqueDomain(db *store.DB, name, owner, base string) string {
	taken := map[string]bool{}
	if fleet, err := db.LoadFleet(); err == nil {
		for i := range fleet.Routes {
			for _, h := range fleet.Routes[i].Hosts {
				taken[strings.ToLower(h)] = true
			}
		}
	}
	if !taken[name+"."+base] {
		return name + "." + base
	}
	return name + "-" + strings.ToLower(owner) + "." + base
}

// freePort primer puerto libre del pool fuera de routes + bind real.
func freePort(db *store.DB) (int, error) {
	from, to := parsePool(db.GetSetting(store.KeyPortPool))
	used := usedPorts(db)
	for p := from; p <= to; p++ {
		if used[p] {
			continue
		}
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		l.Close()
		return p, nil
	}
	return 0, fmt.Errorf("pool %d-%d agotado", from, to)
}

func parsePool(s string) (int, int) {
	parts := strings.Split(s, "-")
	if len(parts) == 2 {
		var from, to int
		fmt.Sscanf(parts[0], "%d", &from)
		fmt.Sscanf(parts[1], "%d", &to)
		if from > 0 && to >= from {
			return from, to
		}
	}
	return 3000, 3999
}

func usedPorts(db *store.DB) map[int]bool {
	out := map[int]bool{}
	fleet, err := db.LoadFleet()
	if err != nil {
		return out
	}
	for i := range fleet.Routes {
		host := fleet.Routes[i].Target
		idx := strings.LastIndex(host, ":")
		if idx < 0 {
			continue
		}
		var p int
		fmt.Sscanf(host[idx+1:], "%d", &p)
		if p > 0 {
			out[p] = true
		}
	}
	return out
}

// Suggestion valores inferidos para `apps add` (wizard los propone).
type Suggestion struct {
	Name    string
	Domain  string
	Port    int
	Preset  string
	Command string
	Dir     string
	// Plan build canónico (buildpack) para este proyecto.
	Plan buildpack.Plan
}

// Suggest infiere desde repo local (disco) o URL https (API pública).
// Nunca falla por red: sin acceso, plan vacío (el wizard pregunta).
func Suggest(db *store.DB, gh *github.Manager, repoURL string) (*Suggestion, error) {
	s := &Suggestion{}
	if st, err := os.Stat(repoURL); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(repoURL)
		s.Dir = abs
		s.Name = slug(filepath.Base(abs))
		if plan, err := buildpack.DetectDir(abs); err == nil {
			s.Plan = plan
			s.Preset = plan.Kind
		}
	} else {
		owner, repo, ok := github.ParseRepo(repoURL)
		if !ok {
			return nil, fmt.Errorf("repo no es dir local ni URL https válida")
		}
		s.Name = slug(repo)
		if gh != nil {
			// público sin token; privado sin App -> plan vacío
			if p, err := detectPlanRemote(gh, owner, repo); err == nil {
				s.Plan = p
				s.Preset = p.Kind
			}
		}
	}
	if binKey, ok := presetBinKey[s.Preset]; ok {
		s.Command = db.GetSetting(binKey)
	}
	if base := db.GetSetting(store.KeyBaseDomain); base != "" && s.Name != "" {
		s.Domain = uniqueDomain(db, s.Name, "", base)
	}
	if port, err := freePort(db); err == nil {
		s.Port = port
	}
	return s, nil
}
