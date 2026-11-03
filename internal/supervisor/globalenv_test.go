package supervisor

import (
	"testing"

	"rkt-proxy/internal/config"
)

// TestGlobalEnvSeFusionaSinMutarManifiesto: las globales entran en el env
// efectivo de cada app, la app gana, y el spec guardado no se toca.
func TestGlobalEnvSeFusionaSinMutarManifiesto(t *testing.T) {
	original := map[string]string{"API_URL": "https://propia.test", "NODE_ENV": "production"}
	spec := config.App{
		Name:    "a",
		Command: "node",
		Dir:     t.TempDir(),
		Env:     original,
	}
	m := New([]config.App{spec}, map[string]string{"API_URL": "https://global.test", "TZ": "UTC"}, t.TempDir(), testLog(t))
	if len(m.apps) != 1 {
		t.Fatalf("apps = %d", len(m.apps))
	}
	env := m.apps[0].cfg.Env
	if env["API_URL"] != "https://propia.test" {
		t.Fatalf("la app debe ganar: %q", env["API_URL"])
	}
	if env["TZ"] != "UTC" || env["NODE_ENV"] != "production" {
		t.Fatalf("faltan globales/propias: %+v", env)
	}
	if original["API_URL"] != "https://propia.test" || len(original) != 2 {
		t.Fatalf("el manifiesto no debe mutarse: %+v", original)
	}
}

// TestApplyDetectaCambioDeGlobal: cambiar una global cambia el env efectivo,
// así que Apply reinicia la app (specsEqual debe verlo).
func TestApplyDetectaCambioDeGlobal(t *testing.T) {
	spec := config.App{Name: "a", Command: "node", Dir: t.TempDir(), Env: map[string]string{"PORT": "3000"}}
	m := New([]config.App{spec}, map[string]string{"TZ": "UTC"}, t.TempDir(), testLog(t))
	before := m.apps[0].cfg
	m.global = map[string]string{"TZ": "Europe/Madrid"}
	after := m.effective(spec)
	if specsEqual(before, after) {
		t.Fatal("un cambio de global debe alterar la spec efectiva")
	}
}
