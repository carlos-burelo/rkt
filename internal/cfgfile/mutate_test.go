package cfgfile

import (
	"testing"

	"rkt-proxy/internal/config"
)

func mutateCfg(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		Apps: []config.App{{
			Name: "w", Command: "cmd.exe", Dir: dir,
			Env:          map[string]string{"PORT": "3001", "KEEP": "1"},
			RestartDelay: "5s", MinUptime: "10s",
		}},
		Routes: []config.Route{
			{Hosts: []string{"w.test"}, Target: "http://127.0.0.1:3001"},
			{Hosts: []string{"other.test"}, Target: "http://127.0.0.1:9999"},
		},
	}
	cfg.ApplyDefaults()
	return cfg
}

func TestUpdateAppCanonico(t *testing.T) {
	cfg := mutateCfg(t)
	cmd, delay, port, domain := "nuevo.exe", "9s", 4000, "w2.test"
	err := UpdateApp(cfg, "w", AppPatch{
		Command: &cmd, RestartDelay: &delay, Port: &port, Domain: &domain,
	})
	if err != nil {
		t.Fatal(err)
	}
	app := FindApp(cfg, "w")
	if app.Command != cmd || app.RestartDelay != delay || app.Env["PORT"] != "4000" {
		t.Fatalf("patch no aplicado: %+v", app)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("rutas: %+v", cfg.Routes)
	}
	found := false
	for _, r := range cfg.Routes {
		if r.Target == "http://127.0.0.1:4000" && len(r.Hosts) == 1 && r.Hosts[0] == domain {
			found = true
		}
	}
	if !found {
		t.Fatalf("dominio no retargetado: %+v", cfg.Routes)
	}
}

func TestUpdateAppValidaAntesDeGuardar(t *testing.T) {
	cfg := mutateCfg(t)
	empty := ""
	if err := UpdateApp(cfg, "w", AppPatch{Command: &empty}); err == nil {
		t.Fatal("command vacío debió fallar")
	}
	if err := UpdateApp(cfg, "nope", AppPatch{}); err == nil {
		t.Fatal("app inexistente debió fallar")
	}
}

func TestRemoveAppRutas(t *testing.T) {
	cfg := mutateCfg(t)
	dropped, err := RemoveApp(cfg, "w", false)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 || len(cfg.Apps) != 0 || len(cfg.Routes) != 1 {
		t.Fatalf("remove: dropped=%d apps=%d routes=%+v", dropped, len(cfg.Apps), cfg.Routes)
	}
	if cfg.Routes[0].Hosts[0] != "other.test" {
		t.Fatalf("ruta ajena tocada: %+v", cfg.Routes)
	}

	cfg = mutateCfg(t)
	dropped, err = RemoveApp(cfg, "w", true)
	if err != nil || dropped != 0 || len(cfg.Routes) != 2 {
		t.Fatalf("keepRoutes: dropped=%d err=%v routes=%+v", dropped, err, cfg.Routes)
	}
}

func TestRouteMutaciones(t *testing.T) {
	cfg := mutateCfg(t)
	if err := AddRoute(cfg, []string{"new.test"}, "http://127.0.0.1:5000"); err != nil {
		t.Fatal(err)
	}
	if err := AddRoute(cfg, []string{"w.test"}, "http://127.0.0.1:5000"); err == nil {
		t.Fatal("host duplicado debió fallar")
	}
	if err := AddRoute(cfg, nil, ""); err == nil {
		t.Fatal("ruta vacía debió fallar")
	}

	ok, err := SetRouteTarget(cfg, "NEW.test", "http://127.0.0.1:5001")
	if err != nil || !ok {
		t.Fatalf("retarget: ok=%v err=%v", ok, err)
	}
	ok, err = SetRouteTarget(cfg, "nope.test", "http://127.0.0.1:5001")
	if err != nil || ok {
		t.Fatalf("retarget inexistente: ok=%v err=%v", ok, err)
	}

	removed, err := RemoveRouteHost(cfg, "W.TEST")
	if err != nil || !removed {
		t.Fatalf("remove host: removed=%v err=%v", removed, err)
	}
	removed, err = RemoveRouteHost(cfg, "nope.test")
	if err != nil || removed {
		t.Fatalf("remove host inexistente: removed=%v err=%v", removed, err)
	}
}

func TestUnsetEnvCanonico(t *testing.T) {
	cfg := mutateCfg(t)
	if err := UnsetEnv(cfg, "w", "KEEP"); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindApp(cfg, "w").Env["KEEP"]; ok {
		t.Fatal("KEEP sigue presente")
	}
	if err := UnsetEnv(cfg, "nope", "K"); err == nil {
		t.Fatal("app inexistente debió fallar")
	}
	if err := UnsetEnv(cfg, "w", ""); err == nil {
		t.Fatal("clave vacía debió fallar")
	}
}
