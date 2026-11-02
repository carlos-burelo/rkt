// Package discover: auto-registro de apps desde repos instalados.
// Repo nuevo + preset conocido = app + route + deploy inmediato.
// Sin preset (stack raro) se avisa con el comando manual.
package discover

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/store"
)

// presetBinKey qué setting de binario usa cada preset.
var presetBinKey = map[string]string{
	"next": "node_bin", "node": "node_bin", "python": "python_bin",
}

// Deps dependencias inyectables (tests usan fakes).
type Deps struct {
	GH       *github.Manager
	DB       *store.DB
	Log      *slog.Logger
	DeployFn func(name string) (string, error)
	// DirBase dir padre de apps (default C:\apps). Tests lo aíslan.
	DirBase string
}

// Run registra repos no conocidos de TODAS las instalaciones (vía API,
// fuente de verdad) y los despliega. Idempotente.
func Run(d Deps) error {
	installs, err := d.GH.Installations()
	if err != nil {
		return fmt.Errorf("instalaciones: %w", err)
	}
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
			return err
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
		}
	}
	return nil
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
	preset, err := detectPreset(d.GH, owner, repo, branch, tok)
	if err != nil {
		return err
	}
	if preset == "" {
		d.Log.Warn("repo sin preset conocido, regístralo manual",
			"repo", owner+"/"+repo, "ayuda", "apps add")
		return nil
	}
	base := d.DB.GetSetting("base_domain")
	if base == "" {
		return fmt.Errorf("sin base_domain (settings set base_domain=...)")
	}
	binKey, ok := presetBinKey[preset]
	if !ok {
		return fmt.Errorf("preset %q sin binario", preset)
	}
	bin := d.DB.GetSetting(binKey)
	if bin == "" {
		return fmt.Errorf("sin %s (settings set %s=...)", binKey, binKey)
	}
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return fmt.Errorf("%s no es ejecutable: %s", binKey, bin)
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
	app := config.App{
		Name: name, Command: bin, Env: map[string]string{"PORT": strconv.Itoa(port)},
		Deploy: &config.Deploy{
			Repo: "https://github.com/" + owner + "/" + repo + ".git",
			Ref:  branch, Auto: true,
		},
	}
	app.SetDelays(5*time.Second, 10*time.Second)
	app.SetDelays(5*time.Second, 10*time.Second)
	dir := d.DirBase
	if dir == "" {
		dir = `C:\apps`
	}
	dir = filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("crear dir: %w", err)
	}
	app.Dir = dir
	switch preset {
	case "next":
		app.Args = []string{`.next\standalone\server.js`}
		app.Deploy.Build = []string{"npm ci --omit=dev", "npm run build"}
	case "node":
		app.Args = []string{"server.js"}
		app.Deploy.Build = []string{"npm ci --omit=dev"}
	case "python":
		app.Args = []string{"-u", "app.py"}
	}
	if err := d.DB.UpsertApp(&app); err != nil {
		return err
	}
	if _, err := d.DB.AddRoute([]string{domain}, fmt.Sprintf("http://127.0.0.1:%d", port)); err != nil {
		return err
	}
	d.Log.Info("app auto-registrada", "app", name, "domain", domain, "port", port)
	if d.DeployFn != nil {
		if rev, err := d.DeployFn(name); err != nil {
			d.Log.Error("auto-deploy inicial fallido", "app", name, "err", err)
		} else {
			d.Log.Info("auto-deploy inicial ok", "app", name, "rev", rev)
		}
	}
	return nil
}

// detectPreset next|node|python por package.json/requirements, "" si nada.
func detectPreset(gh *github.Manager, owner, repo, branch, tok string) (string, error) {
	if content, ok, err := gh.FileContent(owner, repo, "package.json", branch, tok); err != nil {
		return "", err
	} else if ok {
		var pj struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal([]byte(content), &pj); err == nil {
			if _, ok := pj.Dependencies["next"]; ok {
				return "next", nil
			}
			if _, ok := pj.DevDependencies["next"]; ok {
				return "next", nil
			}
			return "node", nil
		}
	}
	if _, ok, err := gh.FileContent(owner, repo, "requirements.txt", branch, tok); err != nil {
		return "", err
	} else if ok {
		return "python", nil
	}
	return "", nil
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
	from, to := parsePool(db.GetSetting("port_pool"))
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
