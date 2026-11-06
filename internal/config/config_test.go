package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func finalized(t *testing.T, cfg *Config) *Config {
	t.Helper()
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return cfg
}

func TestMatchExact(t *testing.T) {
	cfg := finalized(t, &Config{
		ListenHTTP: ":80",
		Routes:     []Route{{Hosts: []string{"app.ejemplo.com"}, Target: "http://127.0.0.1:3000"}},
	})
	if cfg.Match("app.ejemplo.com") == nil {
		t.Fatal("debía matchear exacto")
	}
	if cfg.Match("app.ejemplo.com:80") == nil {
		t.Fatal("debía matchear con puerto")
	}
	if cfg.Match("otro.ejemplo.com") != nil {
		t.Fatal("no debía matchear")
	}
}

func TestMatchWildcard(t *testing.T) {
	cfg := finalized(t, &Config{
		Routes: []Route{
			{Hosts: []string{"app.ejemplo.com"}, Target: "http://127.0.0.1:3000"},
			{Hosts: []string{"*.ejemplo.com"}, Target: "http://127.0.0.1:4000"},
		},
	})
	if u := cfg.Match("app.ejemplo.com"); u == nil || u.Host != "127.0.0.1:3000" {
		t.Fatalf("exacto debe ganar, got %v", u)
	}
	if u := cfg.Match("otro.ejemplo.com"); u == nil || u.Host != "127.0.0.1:4000" {
		t.Fatalf("wildcard debe matchear, got %v", u)
	}
	if cfg.Match("ejemplo.com") != nil {
		t.Fatal("base no debe matchear *.ejemplo.com")
	}
}

func TestMatchCaseInsensitive(t *testing.T) {
	cfg := finalized(t, &Config{
		Routes: []Route{{Hosts: []string{"App.Ejemplo.COM"}, Target: "http://127.0.0.1:3000"}},
	})
	if cfg.Match("app.ejemplo.com") == nil {
		t.Fatal("match debe ser case-insensitive")
	}
}

func TestLiveSwapAndNeedRestart(t *testing.T) {
	a := finalized(t, &Config{
		ListenHTTP: ":80",
		Routes:     []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"}},
	})
	b := finalized(t, &Config{
		ListenHTTP: ":8080",
		Routes:     []Route{{Hosts: []string{"a.com", "b.com"}, Target: "http://127.0.0.1:3000"}},
	})
	live := NewLive(a)
	if live.Get().Match("b.com") != nil {
		t.Fatal("b.com no existe aún")
	}
	live.Swap(b)
	if live.Get().Match("b.com") == nil {
		t.Fatal("tras swap b.com debe resolver")
	}
	need := NeedRestart(a, b)
	if len(need) != 1 || need[0] != "listen_http" {
		t.Fatalf("need restart mal: %v", need)
	}
	if len(NeedRestart(a, a)) != 0 {
		t.Fatal("sin cambios no pide restart")
	}
}

func TestDefaultsHealthzRateLimit(t *testing.T) {
	cfg := Default()
	cfg.Routes = []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"}}
	finalized(t, cfg)
	if !cfg.Healthz.Enabled || cfg.Healthz.Listen != "127.0.0.1:8090" {
		t.Fatalf("defaults healthz mal: %+v", cfg.Healthz)
	}
	if !cfg.RateLimit.Enabled || cfg.RateLimit.RPS != 20 || cfg.RateLimit.Burst != 40 {
		t.Fatalf("defaults rate_limit mal: %+v", cfg.RateLimit)
	}
}

func TestInvalidRateLimitRejected(t *testing.T) {
	cases := []Config{
		{RateLimit: RateLimitConfig{Enabled: true, RPS: 0, Burst: 1}},
		{RateLimit: RateLimitConfig{Enabled: true, RPS: 5, Burst: 0}},
		{Healthz: HealthzConfig{Enabled: true, Listen: ""}},
	}
	for i := range cases {
		cases[i].Routes = []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"}}
		if err := cases[i].Finalize(); err == nil {
			t.Fatalf("caso %d debía fallar", i)
		}
	}
}

func TestDuplicateHostRejected(t *testing.T) {
	cfg := &Config{Routes: []Route{
		{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"},
		{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:4000"},
	}}
	if err := cfg.Finalize(); err == nil {
		t.Fatal("duplicado debe fallar")
	}
}

func TestDenyPathsDefaultsAndInvalid(t *testing.T) {
	cfg := Default()
	cfg.Routes = []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"}}
	finalized(t, cfg)
	found := false
	for _, p := range cfg.DenyPaths {
		if p == "/.env" {
			found = true
		}
	}
	if !found {
		t.Fatalf("defaults deben incluir /.env: %v", cfg.DenyPaths)
	}
	bad := &Config{
		DenyPaths: []string{"/ok/*", "/mal/[/"},
		Routes:    []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"}},
	}
	if err := bad.Finalize(); err == nil {
		t.Fatal("glob inválido debe fallar")
	}
}

func TestAppsValidation(t *testing.T) {
	dir := t.TempDir()
	good := &Config{
		Apps: []App{{
			Name: "web-1", Command: "node", Args: []string{"server.js"}, Dir: dir,
			Env: map[string]string{"PORT": "3001"}, RestartDelay: "5s",
		}},
		Routes: []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3001"}},
	}
	cfg := finalized(t, good)
	if len(cfg.Apps) != 1 || cfg.Apps[0].Delay() != 5*time.Second {
		t.Fatalf("app mal cargada: %+v", cfg.Apps)
	}

	invalid := []*Config{
		{Apps: []App{{Name: "Web", Command: "x", RestartDelay: "5s"}}},                                              // nombre
		{Apps: []App{{Name: "a", Command: "x", RestartDelay: "5s"}, {Name: "a", Command: "y", RestartDelay: "5s"}}}, // duplicado
		{Apps: []App{{Name: "a", RestartDelay: "5s"}}},                                                              // sin command
		{Apps: []App{{Name: "a", Command: "x", Dir: filepath.Join(dir, "nodir"), RestartDelay: "5s"}}},              // dir
		{Apps: []App{{Name: "a", Command: "x", RestartDelay: "nunca"}}},                                             // delay
	}
	for i, c := range invalid {
		if err := c.Finalize(); err == nil {
			t.Fatalf("caso inválido %d debía fallar", i)
		}
	}

	def := finalized(t, &Config{Apps: []App{{Name: "a", Command: "x"}}})
	if def.Apps[0].Delay() != 5*time.Second {
		t.Fatalf("delay default debe ser 5s: %v", def.Apps[0].Delay())
	}
}

func TestAppsEnvFileAndExpand(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "app.env")
	if err := os.WriteFile(envPath, []byte("DESDE_FILE=file\nBASE=base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RKT_DOT_OS", "osval")

	cfg := finalized(t, &Config{
		Apps: []App{{
			Name: "a", Command: "x", EnvFile: envPath,
			Env: map[string]string{"INLINE": "${BASE}-in", "SOLO_OS": "$RKT_DOT_OS"},
		}},
	})
	env := cfg.Apps[0].Env
	if env["DESDE_FILE"] != "file" || env["INLINE"] != "base-in" || env["SOLO_OS"] != "osval" {
		t.Fatalf("merge mal: %v", env)
	}

	over := finalized(t, &Config{
		Apps: []App{{Name: "a", Command: "x", EnvFile: envPath, Env: map[string]string{"BASE": "ganainline"}}},
	})
	if over.Apps[0].Env["BASE"] != "ganainline" {
		t.Fatalf("inline debe ganar a file: %v", over.Apps[0].Env)
	}

	bad := &Config{Apps: []App{{Name: "a", Command: "x", EnvFile: filepath.Join(dir, "falta.env")}}}
	if err := bad.Finalize(); err == nil {
		t.Fatal("env_file inexistente debe fallar")
	}
}

func TestDeployValidation(t *testing.T) {
	dir := t.TempDir()
	base := func(dep *Deploy) *Config {
		return &Config{
			Apps:   []App{{Name: "a", Command: "x", Dir: dir, Deploy: dep}},
			Routes: []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:1"}},
		}
	}
	cfg := finalized(t, base(&Deploy{Repo: "https://github.com/o/r.git"}))
	d := cfg.Apps[0].Deploy
	if d.Ref != "main" || d.TokenEnv != "GITHUB_TOKEN" {
		t.Fatalf("defaults deploy mal: %+v", d)
	}
	for name, dep := range map[string]*Deploy{
		"sin repo":    {Ref: "main"},
		"build vacío": {Repo: "https://github.com/o/r.git", Build: []string{""}},
	} {
		if err := base(dep).Finalize(); err == nil {
			t.Fatalf("%s: debía fallar", name)
		}
	}
	if err := (&Config{
		Apps: []App{{Name: "a", Command: "x", Deploy: &Deploy{Repo: "https://github.com/o/r.git"}}},
	}).Finalize(); err == nil {
		t.Fatal("deploy sin dir debe fallar")
	}
}

func TestSecurityDefaults(t *testing.T) {
	cfg := Default()
	cfg.Routes = []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:3000"}}
	finalized(t, cfg)
	if cfg.MaxBodyMB != 10 {
		t.Fatalf("default max_body_mb debe ser 10: %d", cfg.MaxBodyMB)
	}
	bad := &Config{
		MaxBodyMB: -1,
		Routes:    []Route{{Hosts: []string{"a.com"}, Target: "http://127.0.0.1:1"}},
	}
	if err := bad.Finalize(); err == nil {
		t.Fatal("max_body_mb negativo debe fallar")
	}
}

func TestMergeEnvGlobalYOverride(t *testing.T) {
	cfg := &Config{GlobalEnv: map[string]string{"A": "global", "B": "global"}}
	got := cfg.RuntimeEnv(map[string]string{"B": "propia", "C": "propia"})
	want := map[string]string{"A": "global", "B": "propia", "C": "propia"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, quiere %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, quiere %d", len(got), len(want))
	}
	if cfg.GlobalEnv["B"] != "global" {
		t.Fatal("MergeEnv no debe mutar el global")
	}
	if MergeEnv(nil, nil) != nil {
		t.Fatal("sin entradas debe ser nil")
	}
}

func TestHealthzSoloLoopback(t *testing.T) {
	ok := finalized(t, &Config{Healthz: HealthzConfig{Enabled: true, Listen: "127.0.0.1:8090"}})
	if !ok.Healthz.Enabled {
		t.Fatal("healthz debió quedar activo")
	}
	for _, bad := range []string{"0.0.0.0:8090", ":8090", "192.168.1.5:8090"} {
		c := &Config{Healthz: HealthzConfig{Enabled: true, Listen: bad}}
		if err := c.Finalize(); err == nil {
			t.Fatalf("healthz %q expuesto debe fallar", bad)
		}
	}
}

func TestExportImportJSON(t *testing.T) {
	cfg := finalized(t, &Config{
		Apps:   []App{{Name: "a", Command: "x", Env: map[string]string{"K": "v"}}},
		Routes: []Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:1"}},
	})
	data, err := ExportJSONWithSecrets(cfg, map[string]string{"site_name": "rkt"})
	if err != nil {
		t.Fatal(err)
	}
	back, settings, err := ImportJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Apps) != 1 || len(back.Routes) != 1 || settings["site_name"] != "rkt" {
		t.Fatalf("roundtrip = %+v %v", back, settings)
	}
	if back.Apps[0].Env["K"] != "v" {
		t.Fatalf("export con secretos perdió env: %+v", back.Apps[0].Env)
	}

	// Export por defecto: claves presentes, valores redactados.
	safe, err := ExportJSON(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(safe), SecretMask) || strings.Contains(string(safe), `"K": "v"`) {
		t.Fatalf("export saneado filtró secretos: %s", safe)
	}
	if _, _, err := ImportJSON(safe); err == nil {
		t.Fatal("importar un export redactado sin config previa debe fallar")
	}
	merged, _, err := ImportJSONMerged(safe, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Apps[0].Env["K"] != "v" {
		t.Fatalf("merge de export redactado perdió env: %+v", merged.Apps[0].Env)
	}
	if _, _, err := ImportJSON([]byte("{bad")); err == nil {
		t.Fatal("JSON inválido debe fallar")
	}
	if _, _, err := ImportJSON([]byte(`{"routes":[{"hosts":[""],"target":"mal"}]}`)); err == nil {
		t.Fatal("config inválida debe fallar")
	}
}

// FuzzMatch: hosts arbitrarios no pueden tumbar el matcher ni devolver
// targets que no estén en las rutas.
func FuzzMatch(f *testing.F) {
	for _, s := range []string{"", "a.test", "A.TEST:443", "*", "..", "a.test.", "xn--x.test", "a b"} {
		f.Add(s)
	}
	cfg := &Config{Routes: []Route{
		{Hosts: []string{"a.test", "*.b.test"}, Target: "http://127.0.0.1:3000"},
	}}
	for i := range cfg.Routes {
		if err := cfg.Routes[i].Resolve(); err != nil {
			f.Fatalf("ruta semilla: %v", err)
		}
	}
	f.Fuzz(func(t *testing.T, host string) {
		got := cfg.Match(host)
		if got != nil && got.String() != "http://127.0.0.1:3000" {
			t.Fatalf("target inesperado: %s", got)
		}
	})
}

func TestNormalizeNodePM(t *testing.T) {
	cfg := &Config{Apps: []App{{
		Name: "node", Command: `C:\Program Files\nodejs\npm.cmd`, Args: []string{"run", "start"},
		Deploy: &Deploy{Build: []string{"npm ci --omit=dev", "npm run build", "npx next lint", "pnpm run test"}},
	}}}
	cfg.ApplyDefaults()
	a := cfg.Apps[0]
	if a.Command != "pnpm" {
		t.Fatalf("command = %q", a.Command)
	}
	want := []string{"pnpm install", "pnpm run build", "pnpm exec next lint", "pnpm run test"}
	for i := range want {
		if a.Deploy.Build[i] != want[i] {
			t.Fatalf("build[%d] = %q, quiero %q", i, a.Deploy.Build[i], want[i])
		}
	}
}

func TestGithubPollDefault(t *testing.T) {
	g := &Github{}
	if err := g.EnsureParsed(); err != nil {
		t.Fatal(err)
	}
	if g.Poll() != 10*time.Second {
		t.Fatalf("poll default = %v, quiero 10s", g.Poll())
	}
}

func TestExternalAppValidate(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Apps: []App{{Name: "dev", Runtime: RuntimeExternal, Dir: dir, Env: map[string]string{"PORT": "3007"}}}}
	cfg.ApplyDefaults()
	app := cfg.Apps[0]
	if err := app.ValidateApp(0); err != nil {
		t.Fatalf("externa válida: %v", err)
	}
	bad := app
	bad.Command = "npm"
	if err := bad.ValidateApp(0); err == nil {
		t.Fatal("externa con command debe fallar")
	}
	bad = app
	bad.Dir = ""
	if err := bad.ValidateApp(0); err == nil {
		t.Fatal("externa sin dir debe fallar")
	}
	bad = app
	bad.Env = nil
	if err := bad.ValidateApp(0); err == nil {
		t.Fatal("externa sin PORT debe fallar")
	}
}

func TestPuertoDuplicadoFalla(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Apps: []App{
		{Name: "a", Runtime: RuntimeExternal, Dir: dir, Env: map[string]string{"PORT": "3007"}},
		{Name: "b", Runtime: RuntimeExternal, Dir: dir, Env: map[string]string{"PORT": "3007"}},
	}}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "comparten puerto") {
		t.Fatalf("quiero error de puerto, got %v", err)
	}
}

func TestRouteResolveProtocolos(t *testing.T) {
	cases := []struct{ target, scheme string }{
		{"http://127.0.0.1:3000", "http"},
		{"https://127.0.0.1:3000", "https"},
		{"ws://127.0.0.1:3000", "http"},
		{"wss://127.0.0.1:3000", "https"},
	}
	for _, c := range cases {
		r := Route{Hosts: []string{"a.test"}, Target: c.target}
		if err := r.Resolve(); err != nil {
			t.Fatalf("%s: %v", c.target, err)
		}
		if r.TargetURL().Scheme != c.scheme {
			t.Fatalf("%s scheme = %s", c.target, r.TargetURL().Scheme)
		}
	}
	for _, bad := range []string{"ftp://127.0.0.1:1", "127.0.0.1:3000", "ws://"} {
		r := Route{Hosts: []string{"a.test"}, Target: bad}
		if err := r.Resolve(); err == nil {
			t.Fatalf("%s debía fallar", bad)
		}
	}
}

func TestValidaPersistProteccionYCerts(t *testing.T) {
	base := func() *App {
		return &App{Name: "a", Command: "x", RestartDelay: "5s", MinUptime: "10s"}
	}
	a := base()
	a.Persist = []string{"data", "../fuera"}
	if err := a.ValidateApp(0); err == nil {
		t.Fatal("persist traversal debe fallar")
	}
	a = base()
	a.BasicAuth = "u:plain"
	if err := a.ValidateApp(0); err == nil {
		t.Fatal("basic_auth sin bcrypt debe fallar")
	}
	a = base()
	a.AllowIPs = []string{"nope"}
	if err := a.ValidateApp(0); err == nil {
		t.Fatal("allow_ips inválido debe fallar")
	}
	cfg := Default()
	cfg.TLSCerts = []TLSCert{{Hosts: []string{"a.test"}, CertFile: "no", KeyFile: "no"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("tls_certs inexistente debe fallar")
	}
}
