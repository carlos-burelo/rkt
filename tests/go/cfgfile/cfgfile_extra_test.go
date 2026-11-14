package cfgfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	_, dbCfg, err := LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dbCfg.Close()
	if err := SetGlobalEnv(db, "BASE", "https://api.test"); err != nil {
		t.Fatal(err)
	}
	if err := SetGlobalEnv(db, "", "x"); err == nil {
		t.Fatal("global sin clave debe fallar")
	}
	if err := SetEnv(db, "w", "URL", "${BASE}/v1"); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(db, "nope", "K", "v"); err == nil {
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
	if err := UnsetGlobalEnv(db, "BASE"); err != nil {
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
	if err := db.SetSetting("github_app_id", "42"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting("github_app_slug", "slug"); err != nil {
		t.Fatal(err)
	}
	if err := OverlayGithub(cfg, db); err != nil {
		t.Fatal(err)
	}
	if cfg.Github == nil || cfg.Github.AppID != 42 || cfg.Github.AppSlug != "slug" {
		t.Fatalf("overlay = %+v", cfg.Github)
	}
}

func TestSetAppDomainsYPort(t *testing.T) {
	_, db := testCfg(t)
	cp := &config.Config{
		Apps: []config.App{{Name: "w", Env: map[string]string{"PORT": "3001"}}},
		Routes: []config.Route{
			{Hosts: []string{"viejo.test"}, Target: "http://127.0.0.1:3001"},
			{Hosts: []string{"otro.test"}, Target: "http://127.0.0.1:9999"},
		},
	}
	if err := SetAppDomains(cp, "w", []string{"https://nuevo.test/", "alias.test", "nuevo.test"}); err != nil {
		t.Fatal(err)
	}
	if len(cp.Routes) != 2 || len(cp.Routes[1].Hosts) != 2 || cp.Routes[1].Hosts[0] != "nuevo.test" || cp.Routes[1].Hosts[1] != "alias.test" {
		t.Fatalf("routes = %+v", cp.Routes)
	}
	if err := SetAppDomains(cp, "w", nil); err != nil {
		t.Fatal(err)
	}
	if len(cp.Routes) != 1 {
		t.Fatalf("sin dominio debe quedar solo la otra: %+v", cp.Routes)
	}
	if err := SetAppDomains(cp, "w", []string{"a b"}); err == nil {
		t.Fatal("dominio con espacio debe fallar")
	}
	if err := SetAppDomains(cp, "nope", []string{"x.test"}); err == nil {
		t.Fatal("app inexistente debe fallar")
	}
	// app sin puerto
	cp.Apps = append(cp.Apps, config.App{Name: "sinport"})
	if err := SetAppDomains(cp, "sinport", []string{"x.test"}); err == nil {
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

func TestLoadErrores(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "archivo")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// store.Open falla si el padre es un archivo
	if _, err := Load(filepath.Join(file, "cfg.yaml")); err == nil {
		t.Fatal("Load debía fallar al abrir DB")
	}
	if _, _, err := LoadMerged(filepath.Join(file, "cfg.yaml")); err == nil {
		t.Fatal("LoadMerged debía fallar al abrir DB")
	}

	// infra corrupta: Load y LoadMerged
	cfgPath := filepath.Join(dir, "cfg.yaml")
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveInfra([]byte("{bad")); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Load(cfgPath); err == nil {
		t.Fatal("Load con infra corrupta debía fallar")
	}
	if _, _, err := LoadMerged(cfgPath); err == nil {
		t.Fatal("LoadMerged con infra corrupta debía fallar")
	}
}

func TestLoadMergedSnapshotYOverlayErrores(t *testing.T) {
	cfgPath, db := testCfg(t)
	// Snapshot con DB viva y vacía
	if _, err := Snapshot(db, config.Default()); err != nil {
		t.Fatal(err)
	}
	// Snapshot falla con rutas inválidas en DB
	if err := db.ImportFleet(&config.Config{Routes: []config.Route{{Hosts: []string{""}, Target: "mal"}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadMerged(cfgPath); err == nil {
		t.Fatal("LoadMerged con rutas inválidas debía fallar")
	}
	closed, _ := store.Open(logx.DBPath(filepath.Join(t.TempDir(), "x.yaml")))
	closed.Close()
	if _, err := Snapshot(closed, config.Default()); err == nil {
		t.Fatal("Snapshot con DB cerrada debía fallar")
	}
	// LoadBase con DB cerrada
	if _, err := LoadBase(closed, cfgPath); err == nil {
		t.Fatal("LoadBase con DB cerrada debía fallar")
	}
	// ApplyDev con DB cerrada
	if err := ApplyDev(closed, "127.0.0.1:80", ""); err == nil {
		t.Fatal("ApplyDev con DB cerrada debía fallar")
	}
	// SetGlobalEnv con DB cerrada
	if err := SetGlobalEnv(closed, "K", "v"); err == nil {
		t.Fatal("SetGlobalEnv con DB cerrada debía fallar")
	}
}

func TestAddAppValidateRoutesYSetEnvFile(t *testing.T) {
	cfgPath, db := testCfg(t)
	dir := filepath.Join(filepath.Dir(cfgPath), "app")
	if err := AddApp(db, validApp(t, "w", dir, "3001"), []string{"dup.test"}, "3001"); err != nil {
		t.Fatal(err)
	}
	// ruta duplicada => ValidateRoutes falla
	if err := AddApp(db, validApp(t, "x", dir, "3002"), []string{"dup.test"}, "3002"); err == nil {
		t.Fatal("ruta duplicada debía fallar")
	}
	// env_file inexistente => SetEnv falla
	cfg, db2, err := LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	app := FindApp(cfg, "w")
	app.EnvFile = filepath.Join(dir, "no.env")
	if err := SaveFleet(db2, cfg); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(db2, "w", "K", "v"); err == nil {
		t.Fatal("env_file inexistente debía fallar")
	}
	// env_file válido: las claves entran al scope de expansión
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("FILEVAR=hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.EnvFile = envFile
	if err := SaveFleet(db2, cfg); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(db2, "w", "K", "${FILEVAR}-mundo"); err != nil {
		t.Fatal(err)
	}
	cfg2, db3, err := LoadMerged(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db3.Close()
	if got := FindApp(cfg2, "w").Env["K"]; got != "hola-mundo" {
		t.Fatalf("expansión env_file = %q", got)
	}
}

func TestCreateAppBranches(t *testing.T) {
	_, db := testCfg(t)
	dir := t.TempDir()
	// repo inexistente => Suggest falla
	if _, _, _, err := CreateApp(db, nil, "no-es-un-repo :::", "", "", 0, "", "", false, ""); err == nil {
		t.Fatal("repo inválido debía fallar")
	}
	// dir inexistente
	if _, _, _, err := CreateApp(db, nil, "", "w", "", 0, "", filepath.Join(dir, "nope"), false, ""); err == nil {
		t.Fatal("dir inexistente debía fallar")
	}
	// sin nombre ni inferencia
	if _, _, _, err := CreateApp(db, nil, "", "", "", 0, "", "", false, ""); err == nil {
		t.Fatal("sin nombre debía fallar")
	}
	// dir por defecto + sin plan
	if _, _, _, err := CreateApp(db, nil, "", "soloname", "", 0, "", "", false, ""); err == nil {
		t.Fatal("sin plan debía fallar")
	}
	// sitio estático local: plan static, comando propio y args __static
	site := filepath.Join(dir, "site")
	if err := os.MkdirAll(site, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(site, "index.html"), []byte("<html>x</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	app, domains, portS, err := CreateApp(db, nil, "", "sitio", "sitio.localhost", 4321, "", site, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Static == "" || len(app.Args) != 2 || app.Args[0] != "__static" || app.Env["PORT"] != "4321" {
		t.Fatalf("app estática = %+v", app)
	}
	if len(domains) != 1 || domains[0] != "sitio.localhost" || portS != "4321" {
		t.Fatalf("domains = %v port = %q", domains, portS)
	}
	// comando explícito y sin repo: no crea Deploy
	app2, _, _, err := CreateApp(db, nil, "", "manual", "", 0, "cmd.exe", site, false, "")
	if err != nil || app2.Deploy != nil {
		t.Fatalf("manual = %+v %v", app2, err)
	}
}

func TestCreateAppRepoDominioYExec(t *testing.T) {
	_, db := testCfg(t)
	// servidor público con base_domain: la sugerencia usa el dominio base
	if err := db.SetSetting(store.KeyBaseDomain, "ejemplo.com"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(store.KeyPublicIP, "203.0.113.7"); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"dependencies":{"next":"14.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(store.KeyNodeBin, "node"); err != nil {
		t.Fatal(err)
	}
	app, domains, portS, err := CreateApp(db, nil, repo, "", "", 0, "", "", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Deploy == nil || app.Deploy.Repo != repo {
		t.Fatalf("deploy = %+v", app.Deploy)
	}
	if len(domains) != 1 || !strings.Contains(domains[0], "ejemplo.com") {
		t.Fatalf("dominio sugerido = %v", domains)
	}
	if portS == "" {
		t.Fatal("puerto inferido vacío")
	}
	// fallo al resolver el binario propio en un plan estático
	site := filepath.Join(t.TempDir(), "site")
	_ = os.MkdirAll(site, 0o755)
	_ = os.WriteFile(filepath.Join(site, "index.html"), []byte("x"), 0o644)
	old := execOwn
	execOwn = func() (string, error) { return "", errors.New("sin exe") }
	t.Cleanup(func() { execOwn = old })
	if _, _, _, err := CreateApp(db, nil, "", "s", "", 0, "", site, false, ""); err == nil {
		t.Fatal("CreateApp debía fallar al resolver el ejecutable")
	}
	// MkdirAll falla si un componente de la ruta es un archivo (portable).
	execOwn = old
	blocker := filepath.Join(t.TempDir(), "bloqueo")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := CreateApp(db, nil, "", "bad", "", 0, "cmd.exe", filepath.Join(blocker, "app"), false, ""); err == nil {
		t.Fatal("CreateApp debía fallar al crear el directorio")
	}
}

func TestOverlayGithubPollInvalido(t *testing.T) {
	_, db := testCfg(t)
	if err := db.SetSetting("github_app_id", "5"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Github: &config.Github{PollInterval: "1s"}}
	if err := OverlayGithub(cfg, db); err == nil {
		t.Fatal("poll_interval inválido debía fallar en overlay")
	}
}

func TestRamasCfgfileRestantes(t *testing.T) {
	cfgPath, db := testCfg(t)
	closed, err := store.Open(logx.DBPath(filepath.Join(t.TempDir(), "c.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	// AddApp con DB cerrada
	if err := AddApp(closed, validApp(t, "a", filepath.Join(filepath.Dir(cfgPath), "a"), "3001"), nil, ""); err == nil {
		t.Fatal("AddApp con DB cerrada debía fallar")
	}
	// SetEnv con DB cerrada
	if err := SetEnv(closed, "a", "K", "v"); err == nil {
		t.Fatal("SetEnv con DB cerrada debía fallar")
	}
	// SetEnv con app inexistente
	if err := SetEnv(db, "b", "K", "v"); err == nil {
		t.Fatal("SetEnv con app inexistente debía fallar")
	}
	// SetAppDomains con host que url.Parse no puede normalizar
	cp := &config.Config{Apps: []config.App{{Name: "w", Env: map[string]string{"PORT": "3001"}}}}
	if err := SetAppDomains(cp, "w", []string{"a.test#x"}); err == nil {
		t.Fatal("host con fragmento debía fallar")
	}
	// LoadBase: infra válida en JSON pero inválida para Finalize
	if err := db.SaveInfra([]byte(`{"healthz":{"enabled":true,"listen":"0.0.0.0:1"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBase(db, cfgPath); err == nil {
		t.Fatal("LoadBase con healthz expuesto debía fallar")
	}
	// ApplyDev con infra corrupta
	if err := db.SaveInfra([]byte("{bad")); err != nil {
		t.Fatal(err)
	}
	if err := ApplyDev(db, "127.0.0.1:80", ""); err == nil {
		t.Fatal("ApplyDev con infra corrupta debía fallar")
	}
}
