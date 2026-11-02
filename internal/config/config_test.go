package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testConfig(t *testing.T, content string) *Config {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestMatchExact(t *testing.T) {
	cfg := testConfig(t, `
listen_http: ":80"
routes:
  - hosts: ["app.ejemplo.com"]
    target: "http://127.0.0.1:3000"
`)
	if cfg.Match("app.ejemplo.com") == nil {
		t.Fatal("debió matchear exacto")
	}
	if cfg.Match("app.ejemplo.com:80") == nil {
		t.Fatal("debió matchear con puerto")
	}
	if cfg.Match("otro.ejemplo.com") != nil {
		t.Fatal("no debió matchear")
	}
}

func TestMatchWildcard(t *testing.T) {
	cfg := testConfig(t, `
routes:
  - hosts: ["app.ejemplo.com"]
    target: "http://127.0.0.1:3000"
  - hosts: ["*.ejemplo.com"]
    target: "http://127.0.0.1:4000"
`)
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
	cfg := testConfig(t, `
routes:
  - hosts: ["App.Ejemplo.COM"]
    target: "http://127.0.0.1:3000"
`)
	if cfg.Match("app.ejemplo.com") == nil {
		t.Fatal("match debe ser case-insensitive")
	}
}

func TestLiveSwapAndNeedRestart(t *testing.T) {
	a := testConfig(t, `
listen_http: ":80"
routes:
  - hosts: ["a.com"]
    target: "http://127.0.0.1:3000"
`)
	b := testConfig(t, `
listen_http: ":8080"
routes:
  - hosts: ["a.com", "b.com"]
    target: "http://127.0.0.1:3000"
`)
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
	cfg := testConfig(t, `
routes:
  - hosts: ["a.com"]
    target: "http://127.0.0.1:3000"
`)
	if !cfg.Healthz.Enabled || cfg.Healthz.Listen != "127.0.0.1:8090" {
		t.Fatalf("defaults healthz mal: %+v", cfg.Healthz)
	}
	if !cfg.RateLimit.Enabled || cfg.RateLimit.RPS != 20 || cfg.RateLimit.Burst != 40 {
		t.Fatalf("defaults rate_limit mal: %+v", cfg.RateLimit)
	}
}

func TestInvalidRateLimitRejected(t *testing.T) {
	for _, body := range []string{
		"rate_limit:\n  enabled: true\n  requests_per_second: 0\n  burst: 1\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:3000\n",
		"rate_limit:\n  enabled: true\n  requests_per_second: 5\n  burst: 0\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:3000\n",
		"healthz:\n  enabled: true\n  listen: \"\"\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:3000\n",
	} {
		f, _ := os.CreateTemp("", "rkt-*.yaml")
		f.WriteString(body)
		f.Close()
		if _, err := Load(f.Name()); err == nil {
			t.Fatalf("debió fallar: %q", body)
		}
		os.Remove(f.Name())
	}
}

func TestDuplicateHostRejected(t *testing.T) {
	f, _ := os.CreateTemp("", "rkt-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString(`
routes:
  - hosts: ["a.com"]
    target: "http://127.0.0.1:3000"
  - hosts: ["a.com"]
    target: "http://127.0.0.1:4000"
`)
	f.Close()
	if _, err := Load(f.Name()); err == nil {
		t.Fatal("duplicado debe fallar")
	}
}

func TestDenyPathsDefaultsAndInvalid(t *testing.T) {
	cfg := testConfig(t, `
routes:
  - hosts: ["a.com"]
    target: "http://127.0.0.1:3000"
`)
	found := false
	for _, p := range cfg.DenyPaths {
		if p == "/.env" {
			found = true
		}
	}
	if !found {
		t.Fatalf("defaults deben incluir /.env: %v", cfg.DenyPaths)
	}

	f, _ := os.CreateTemp("", "rkt-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString("deny_paths:\n  - \"/ok/*\"\n  - \"/mal/[/\"\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:3000\n")
	f.Close()
	if _, err := Load(f.Name()); err == nil {
		t.Fatal("glob inválido debe fallar")
	}
}

func TestAppsValidation(t *testing.T) {
	dir := t.TempDir()
	good := "apps:\n  - name: web-1\n    command: node\n    args: [\"server.js\"]\n    dir: '" + dir + "'\n    env:\n      PORT: \"3001\"\n    restart_delay: 5s\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:3001\n"
	cfg := testConfig(t, good)
	if len(cfg.Apps) != 1 || cfg.Apps[0].Delay() != 5*time.Second {
		t.Fatalf("app mal cargada: %+v", cfg.Apps)
	}

	cases := map[string]string{"nombre mayúsculas": "apps:\n  - name: Web\n    command: x\n    restart_delay: 5s\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n",
		"nombre duplicado":  "apps:\n  - name: a\n    command: x\n    restart_delay: 5s\n  - name: a\n    command: y\n    restart_delay: 5s\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n",
		"sin command":       "apps:\n  - name: a\n    restart_delay: 5s\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n",
		"dir inexistente":   "apps:\n  - name: a\n    command: x\n    dir: '" + dir + "\\nodir-rkt'\n    restart_delay: 5s\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n",
		"delay inválido":    "apps:\n  - name: a\n    command: x\n    restart_delay: nunca\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n",
		"delay por defecto": "",
	}
	for name, body := range cases {
		if body == "" {
			continue
		}
		f, _ := os.CreateTemp("", "rkt-*.yaml")
		f.WriteString(body)
		f.Close()
		if _, err := Load(f.Name()); err == nil {
			t.Fatalf("%s: debió fallar", name)
		}
		os.Remove(f.Name())
	}

	def := testConfig(t, "apps:\n  - name: a\n    command: x\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n")
	if def.Apps[0].Delay() != 5*time.Second {
		t.Fatalf("delay default debe ser 5s: %v", def.Apps[0].Delay())
	}
}

func TestAppsEnvFileAndExpand(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "app.env")
	os.WriteFile(envPath, []byte("DESDE_FILE=file\nBASE=base\n"), 0o644)
	os.Setenv("RKT_DOT_OS", "osval")
	defer os.Unsetenv("RKT_DOT_OS")

	cfg := testConfig(t, "apps:\n  - name: a\n    command: x\n    env_file: '"+envPath+"'\n    env:\n      INLINE: \"${BASE}-in\"\n      SOLO_OS: \"$RKT_DOT_OS\"\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n")
	env := cfg.Apps[0].Env
	if env["DESDE_FILE"] != "file" || env["INLINE"] != "base-in" || env["SOLO_OS"] != "osval" {
		t.Fatalf("merge mal: %v", env)
	}

	over := testConfig(t, "apps:\n  - name: a\n    command: x\n    env_file: '"+envPath+"'\n    env:\n      BASE: ganainline\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n")
	if over.Apps[0].Env["BASE"] != "ganainline" {
		t.Fatalf("inline debe ganar a file: %v", over.Apps[0].Env)
	}

	f, _ := os.CreateTemp("", "rkt-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString("apps:\n  - name: a\n    command: x\n    env_file: '" + filepath.Join(dir, "falta.env") + "'\nroutes:\n  - hosts: [a.com]\n    target: http://127.0.0.1:1\n")
	f.Close()
	if _, err := Load(f.Name()); err == nil {
		t.Fatal("env_file inexistente debe fallar")
	}
}
