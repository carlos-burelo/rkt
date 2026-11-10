package cfgfile

import (
	"testing"

	"rkt-proxy/internal/config"
)

func envCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		GlobalEnv: map[string]string{"KEEP": "1", "DROP": "2"},
		Apps: []config.App{{
			Name: "w", Command: "c", Dir: t.TempDir(),
			Env:          map[string]string{"PORT": "3001", "V": "old"},
			RestartDelay: "5s", MinUptime: "10s", Runtime: config.RuntimeManaged,
		}},
	}
}

func TestApplyEnvPatchGlobalYApp(t *testing.T) {
	cfg := envCfg(t)
	if err := ApplyEnvPatch(cfg, "", map[string]string{"NEW": "${KEEP}-x"}, []string{"DROP"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.GlobalEnv["DROP"]; ok || cfg.GlobalEnv["NEW"] == "" {
		t.Fatalf("global = %+v", cfg.GlobalEnv)
	}
	if err := ApplyEnvPatch(cfg, "w", map[string]string{"V": "new", "A": "1"}, []string{"OLD"}); err != nil {
		t.Fatal(err)
	}
	app := FindApp(cfg, "w")
	if app.Env["V"] != "new" || app.Env["A"] != "1" || app.Env["PORT"] != "3001" {
		t.Fatalf("env = %+v", app.Env)
	}
	if err := ApplyEnvPatch(cfg, "nope", map[string]string{"A": "1"}, nil); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
}

func TestApplyEnvPatchRechazaClavesInvalidas(t *testing.T) {
	cfg := envCfg(t)
	for _, k := range []string{"", "lower", "1BAD", "WITH SPACE", "PORT"} {
		if err := ApplyEnvPatch(cfg, "w", map[string]string{k: "x"}, nil); err == nil {
			t.Fatalf("clave %q debía fallar", k)
		}
	}
	if cfg.Apps[0].Env["V"] != "old" {
		t.Fatal("un patch inválido no debe mutar la app")
	}
	if err := ApplyEnvPatch(cfg, "", map[string]string{"OK": "1"}, []string{"lower"}); err == nil {
		t.Fatal("unset inválido debe fallar")
	}
	if cfg.GlobalEnv["OK"] != "" {
		t.Fatal("un patch inválido no debe mutar globales")
	}
}
