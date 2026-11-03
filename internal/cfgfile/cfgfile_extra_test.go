package cfgfile

import (
	"os"
	"path/filepath"
	"testing"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func testCfg(t *testing.T) (string, *store.DB) {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "rkt.test.yaml")
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return cfgPath, db
}

func validApp(t *testing.T, name, dir, port string) config.App {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return config.App{
		Name: name, Command: "node", Dir: dir,
		Env: map[string]string{"PORT": port},
	}
}

func TestAddAppYDuplicados(t *testing.T) {
	cfgPath, db := testCfg(t)
	dir := filepath.Join(filepath.Dir(cfgPath), "app")
	app := validApp(t, "w", dir, "3001")
	if err := AddApp(db, app, []string{"w.test"}, "3001"); err != nil {
		t.Fatal(err)
	}
	if err := AddApp(db, app, nil, ""); err == nil {
		t.Fatal("duplicado debe fallar")
	}
	if err := AddApp(db, validApp(t, "x", dir, "3002"), []string{"x.test"}, ""); err == nil {
		t.Fatal("dominio sin port debe fallar")
	}
	// app inválida (sin command)
	bad := validApp(t, "z", dir, "3003")
	bad.Command = ""
	if err := AddApp(db, bad, nil, ""); err == nil {
		t.Fatal("app sin command debe fallar")
	}
	fleet, err := db.LoadFleet()
	if err != nil || len(fleet.Apps) != 1 || len(fleet.Routes) != 1 {
		t.Fatalf("fleet = %+v %v", fleet, err)
	}
}

func TestSetEnvYGlobales(t *testing.T) {
	cfgPath, db := testCfg(t)
	dir := filepath.Join(filepath.Dir(cfgPath), "app")
	if err := AddApp(db, validApp(t, "w", dir, "3001"), nil, ""); err != nil {
		t.Fatal(err)
	}
	cfg, dbCfg, err := LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbCfg.Close()
	if err := SetGlobalEnv(db, cfg, "BASE", "https://api.test"); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalEnv(db, cfg, "", "x"); err == nil {
		t.Fatal("global sin clave debe fallar")
	}
	if err := SetEnv(db, cfg, "w", "URL", "${BASE}/v1"); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(db, cfg, "nope", "K", "v"); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
	cfg2, db2, err := LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	app := FindApp(cfg2, "w")
	if app.Env["URL"] != "https://api.test/v1" {
		t.Fatalf("expansión con global = %q", app.Env["URL"])
	}
	if FindApp(cfg2, "nope") != nil {
		t.Fatal("FindApp inexistente debe ser nil")
	}
	if err := UnsetGlobalEnv(db, cfg2, "BASE"); err != nil {
		t.Fatal(err)
	}
	cfg3, _ := Load(cfgPath)
	if cfg3.GlobalEnv["BASE"] != "" {
		t.Fatal("UnsetGlobalEnv no borró")
	}
}

func TestLoadMergedSnapshotSaveFleet(t *testing.T) {
	cfgPath, db := testCfg(t)
	dir := filepath.Join(filepath.Dir(cfgPath), "app")
	app := validApp(t, "w", dir, "3001")
	cfg := &config.Config{Apps: []config.App{app}, Routes: []config.Route{{Hosts: []string{"w.test"}, Target: "http://127.0.0.1:3001"}}}
	if err := SaveFleet(db, cfg); err != nil {
		t.Fatal(err)
	}
	merged, db2, err := LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	db2.Close()
	if len(merged.Apps) != 1 || len(merged.Routes) != 1 {
		t.Fatalf("merged = %+v", merged)
	}
	base, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := Snapshot(db, base)
	if err != nil || len(snap.Apps) != 1 {
		t.Fatalf("snapshot = %+v %v", snap, err)
	}
	// SaveFleet inválida no persiste
	bad := &config.Config{Apps: []config.App{{Name: "x"}}}
	if err := SaveFleet(db, bad); err == nil {
		t.Fatal("SaveFleet inválida debe fallar")
	}
	// Snapshot con rutas corruptas en DB
	if err := db.ImportFleet(&config.Config{Routes: []config.Route{{Hosts: []string{""}, Target: "mal"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Snapshot(db, base); err == nil {
		t.Fatal("Snapshot con rutas inválidas debe fallar")
	}
}

func TestOverlayGithubYSeed(t *testing.T) {
	_, db := testCfg(t)
	cfg := &config.Config{}
	if err := OverlayGithub(cfg, db); err != nil || cfg.Github != nil {
		t.Fatalf("sin app_id no debe tocar: %+v %v", cfg.Github, err)
	}
	if itoaOrEmpty(0) != "" || itoaOrEmpty(7) != "7" {
		t.Fatal("itoaOrEmpty mal")
	}
	seedGithubSettings(db, &config.Github{AppID: 42, AppSlug: "slug", WebhookSecret: "s"})
	if db.GetSetting("github_app_id") != "42" || db.GetSetting("github_app_slug") != "slug" {
		t.Fatal("seed no escribió")
	}
	if err := OverlayGithub(cfg, db); err != nil {
		t.Fatal(err)
	}
	if cfg.Github == nil || cfg.Github.AppID != 42 || cfg.Github.AppSlug != "slug" {
		t.Fatalf("overlay = %+v", cfg.Github)
	}
}

func TestSetAppDomainYPort(t *testing.T) {
	_, db := testCfg(t)
	cp := &config.Config{
		Apps: []config.App{{Name: "w", Env: map[string]string{"PORT": "3001"}}},
		Routes: []config.Route{
			{Hosts: []string{"viejo.test"}, Target: "http://127.0.0.1:3001"},
			{Hosts: []string{"otro.test"}, Target: "http://127.0.0.1:9999"},
		},
	}
	if err := SetAppDomain(cp, "w", "https://nuevo.test/"); err != nil {
		t.Fatal(err)
	}
	if len(cp.Routes) != 2 || cp.Routes[1].Hosts[0] != "nuevo.test" {
		t.Fatalf("routes = %+v", cp.Routes)
	}
	if err := SetAppDomain(cp, "w", ""); err != nil {
		t.Fatal(err)
	}
	if len(cp.Routes) != 1 {
		t.Fatalf("sin dominio debe quedar solo la otra: %+v", cp.Routes)
	}
	if err := SetAppDomain(cp, "w", "a b"); err == nil {
		t.Fatal("dominio con espacio debe fallar")
	}
	if err := SetAppDomain(cp, "nope", "x.test"); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
	// app sin puerto
	cp.Apps = append(cp.Apps, config.App{Name: "sinport"})
	if err := SetAppDomain(cp, "sinport", "x.test"); err == nil {
		t.Fatal("app sin puerto debe fallar")
	}
	if err := SetAppPort(cp, "w", 0); err == nil {
		t.Fatal("puerto 0 debe fallar")
	}
	if err := SetAppPort(cp, "nope", 3001); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
	cp.Routes = append(cp.Routes, config.Route{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:3001"})
	if err := SetAppPort(cp, "w", 4000); err != nil {
		t.Fatal(err)
	}
	for _, r := range cp.Routes {
		if r.Hosts[0] == "a.test" && r.Target != "http://127.0.0.1:4000" {
			t.Fatalf("retarget = %+v", r)
		}
	}
	// app sin Env: SetAppPort la crea
	cp.Apps[0].Env = nil
	if err := SetAppPort(cp, "w", 5000); err != nil || cp.Apps[0].Env["PORT"] != "5000" {
		t.Fatalf("Env nil = %+v %v", cp.Apps[0].Env, err)
	}
	if processBin("node", db) != "node" {
		t.Fatal("processBin sin setting debe devolver igual")
	}
	if err := db.SetSetting(store.KeyNodeBin, `C:\node.exe`); err != nil {
		t.Fatal(err)
	}
	if got := processBin("node", db); got != `C:\node.exe` {
		t.Fatalf("processBin node = %q", got)
	}
	if err := db.SetSetting(store.KeyPythonBin, `C:\python.exe`); err != nil {
		t.Fatal(err)
	}
	if got := processBin("python3", db); got != `C:\python.exe` {
		t.Fatalf("processBin python3 = %q", got)
	}
	if got := processBin("go", db); got != "go" {
		t.Fatalf("processBin go = %q", got)
	}
}

func TestLoadBaseCorruptaYApplyDev(t *testing.T) {
	cfgPath, db := testCfg(t)
	if err := db.SaveInfra([]byte("{no-json")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBase(db, cfgPath); err == nil {
		t.Fatal("infra corrupta debe fallar")
	}
	// SaveInfra con DB cerrada (rama de error)
	db.Close()
	if err := SaveInfra(db, config.Default()); err == nil {
		t.Fatal("SaveInfra cerrada debe fallar")
	}
	db2, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := SaveInfra(db2, config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDev(db2, "127.0.0.1:80", "127.0.0.1:443"); err != nil {
		t.Fatal(err)
	}
	base, err := LoadBase(db2, cfgPath)
	if err != nil || !base.LocalCA || base.ListenHTTPS != "127.0.0.1:443" {
		t.Fatalf("ApplyDev = %+v %v", base, err)
	}
}
