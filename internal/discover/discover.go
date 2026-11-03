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
	"next": store.KeyNodeBin, "node": store.KeyNodeBin, "python": store.KeyPythonBin,
}

// PresetBuild tabla canónica next|node|python -> build/args/env.
// Unico lugar con estos literales; CLI/MCP/discover la usan.
func PresetBuild(preset, port string) (build, args []string, env map[string]string) {
	env = map[string]string{}
	switch preset {
	case "next":
		args = []string{`.next\standalone\server.js`}
		build = []string{"npm ci --omit=dev", "npm run build"}
	case "node":
		args = []string{"server.js"}
		build = []string{"npm ci --omit=dev"}
	case "python":
		args = []string{"-u", "app.py"}
	default:
		return nil, nil, env
	}
	if port != "" {
		env["PORT"] = port
	}
	return build, args, env
}

// Classify preset desde contenido package.json + presencia requirements.
// Canónica; detectPreset/detectPresetRemote/sniffLocal delegan aquí.
func Classify(pkgJSON []byte, hasRequirements bool) string {
	if len(pkgJSON) > 0 {
		var pj struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal(pkgJSON, &pj); err == nil {
			if _, ok := pj.Dependencies["next"]; ok {
				return "next"
			}
			if _, ok := pj.DevDependencies["next"]; ok {
				return "next"
			}
			return "node"
		}
	}
	if hasRequirements {
		return "python"
	}
	return ""
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
	preset, err := detectPreset(d.GH, owner, repo, branch, tok)
	if err != nil {
		return err
	}
	if preset == "" {
		d.Log.Warn("repo sin preset conocido, regístralo manual",
			"repo", owner+"/"+repo, "ayuda", "apps add")
		return nil
	}
	base := d.DB.GetSetting(store.KeyBaseDomain)
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
	dir := d.DirBase
	if dir == "" {
		dir = `C:\apps`
	}
	dir = filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("crear dir: %w", err)
	}
	app.Dir = dir
	defBuild, defArgs, _ := PresetBuild(preset, strconv.Itoa(port))
	app.Args = defArgs
	app.Deploy.Build = defBuild
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

// detectPreset next|node|python por package.json/requirements, "" si nada.
func detectPreset(gh *github.Manager, owner, repo, branch, tok string) (string, error) {
	if content, ok, err := gh.FileContent(owner, repo, "package.json", branch, tok); err != nil {
		return "", err
	} else if ok {
		if p := Classify([]byte(content), false); p != "" {
			return p, nil
		}
		// package.json ilegible: cae a requirements
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
}

// Suggest infiere desde repo local (disco) o URL https (API pública).
// Nunca falla por red: sin acceso, preset vacío (el wizard pregunta).
func Suggest(db *store.DB, gh *github.Manager, repoURL string) (*Suggestion, error) {
	s := &Suggestion{}
	if st, err := os.Stat(repoURL); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(repoURL)
		s.Dir = abs
		s.Name = slug(filepath.Base(abs))
		s.Preset = sniffLocal(abs)
	} else {
		owner, repo, ok := github.ParseRepo(repoURL)
		if !ok {
			return nil, fmt.Errorf("repo no es dir local ni URL https válida")
		}
		s.Name = slug(repo)
		if gh != nil {
			// público sin token; privado sin App -> preset vacío
			if p, err := detectPresetRemote(gh, owner, repo); err == nil {
				s.Preset = p
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

// detectPresetRemote package.json público (sin token).
func detectPresetRemote(gh *github.Manager, owner, repo string) (string, error) {
	branch := "main"
	if content, ok, err := gh.FileContent(owner, repo, "package.json", branch, ""); err != nil {
		return "", err
	} else if ok {
		if p := Classify([]byte(content), false); p != "" {
			return p, nil
		}
	}
	if _, ok, err := gh.FileContent(owner, repo, "requirements.txt", branch, ""); err != nil {
		return "", err
	} else if ok {
		return "python", nil
	}
	return "", nil
}

// sniffLocal preset por archivos en disco.
func sniffLocal(dir string) string {
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	_, reqErr := os.Stat(filepath.Join(dir, "requirements.txt"))
	return Classify(data, reqErr == nil)
}
