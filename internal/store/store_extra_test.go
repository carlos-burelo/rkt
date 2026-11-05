package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

func newDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSettingsYVersion(t *testing.T) {
	db := newDB(t)
	if got := db.GetSetting(KeySiteName); got != "" {
		t.Fatalf("setting vacío = %q", got)
	}
	v1 := db.ConfigVersion()
	if err := db.SetSetting(KeySiteName, "rkt"); err != nil {
		t.Fatal(err)
	}
	if db.GetSetting(KeySiteName) != "rkt" || db.ConfigVersion() <= v1 {
		t.Fatalf("setting/version mal: %q v=%d", db.GetSetting(KeySiteName), db.ConfigVersion())
	}
	if len(KnownSettings()) != 8 {
		t.Fatalf("KnownSettings = %v", KnownSettings())
	}
}

func TestFleetRoundtripYColisiones(t *testing.T) {
	db := newDB(t)
	a := &config.App{
		Name: "w", Command: "node", Args: []string{"server.js"},
		Dir: "C:/apps/w", EnvFile: ".env", Env: map[string]string{"PORT": "3000"},
		MemoryMB:   512,
		Disabled:   true,
		HealthPath: "/health",
		Static:     "dist",
		Cron: []config.CronJob{{
			Name: "backup", Schedule: "0 3 * * *", Command: "node backup.js", Timeout: "20m",
		}},
		Deploy: &config.Deploy{Repo: "https://github.com/x/y", Ref: "main", TokenEnv: "T", Build: []string{"pnpm build"}, Auto: true},
	}
	a.SetDelays(2*time.Second, 10*time.Second)
	if err := db.UpsertApp(a); err != nil {
		t.Fatal(err)
	}
	id, err := db.AddRoute([]string{"w.test"}, "http://127.0.0.1:3000")
	if err != nil || id == 0 {
		t.Fatalf("AddRoute = %d %v", id, err)
	}
	fleet, err := db.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	if len(fleet.Apps) != 1 || len(fleet.Routes) != 1 {
		t.Fatalf("fleet = %+v", fleet)
	}
	got := fleet.Apps[0]
	if got.Deploy == nil || got.Deploy.Repo != a.Deploy.Repo || !got.Deploy.Auto || len(got.Deploy.Build) != 1 {
		t.Fatalf("deploy = %+v", got.Deploy)
	}
	if got.Env["PORT"] != "3000" || got.Delay() != 2*time.Second || got.MinUp() != 10*time.Second {
		t.Fatalf("app = %+v", got)
	}
	if !got.Disabled || got.HealthPath != "/health" || got.Static != "dist" {
		t.Fatalf("estado = %+v", got)
	}
	if len(got.Cron) != 1 || got.Cron[0].Name != "backup" || got.Cron[0].Schedule != "0 3 * * *" ||
		got.Cron[0].Command != "node backup.js" || got.Cron[0].Timeout != "20m" {
		t.Fatalf("cron = %+v", got.Cron)
	}

	// Upsert actualiza estado y cron (no solo inserta).
	a.Disabled = false
	a.Cron = append(a.Cron, config.CronJob{Name: "cleanup", Schedule: "*/5 * * * *", Command: "node clean.js"})
	if err := db.UpsertApp(a); err != nil {
		t.Fatal(err)
	}
	fleet, err = db.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	got = fleet.Apps[0]
	if got.Disabled || len(got.Cron) != 2 || got.Cron[1].Name != "cleanup" {
		t.Fatalf("upsert = %+v", got)
	}

	// ImportFleet reemplaza apps y rutas
	cfg := &config.Config{
		Apps:   []config.App{{Name: "b", Command: "x", Env: map[string]string{}}},
		Routes: []config.Route{{Hosts: []string{"b.test"}, Target: "http://127.0.0.1:3001"}},
	}
	if err := db.ImportFleet(cfg); err != nil {
		t.Fatal(err)
	}
	fleet, _ = db.LoadFleet()
	if len(fleet.Apps) != 1 || fleet.Apps[0].Name != "b" || fleet.Routes[0].Hosts[0] != "b.test" {
		t.Fatalf("import = %+v", fleet)
	}
}

func TestAlertsCRUD(t *testing.T) {
	db := newDB(t)
	for _, bad := range []struct {
		kind string
		th   float64
		win  int
	}{{"otro", 1, 60}, {"failed", 0, 60}, {"failed", 1, 4}, {"failed", 1, 2000}} {
		if _, err := db.AddRule("a", bad.kind, bad.th, bad.win); err == nil {
			t.Fatalf("regla inválida %+v debe fallar", bad)
		}
	}
	id, err := db.AddRule("", "failed", 2, 60) // app vacía => *
	if err != nil {
		t.Fatal(err)
	}
	rules, err := db.ListRules()
	if err != nil || len(rules) != 1 || rules[0].App != "*" || rules[0].Kind != "failed" {
		t.Fatalf("rules = %+v %v", rules, err)
	}
	if err := db.MarkFired(id, time.Now()); err != nil {
		t.Fatal(err)
	}
	rules, _ = db.ListRules()
	if rules[0].LastFired == "" {
		t.Fatal("MarkFired no persistió")
	}
	if err := db.DeleteRule(id); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRule(id); err == nil {
		t.Fatal("borrar regla inexistente debe fallar")
	}
}

func TestRuntimeExternalRoundtrip(t *testing.T) {
	db := newDB(t)
	dir := t.TempDir()
	app := config.App{Name: "ext", Runtime: config.RuntimeExternal, Dir: dir, Env: map[string]string{"PORT": "3999"}}
	app.SetDelays(time.Second, time.Second)
	if err := db.UpsertApp(&app); err != nil {
		t.Fatal(err)
	}
	f, err := db.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Apps) != 1 || !f.Apps[0].External() || f.Apps[0].Dir != dir {
		t.Fatalf("roundtrip externa: %+v", f.Apps)
	}
}

func TestPruneYDeployments(t *testing.T) {
	db := newDB(t)
	oldID, err := db.InsertDeployment("w", "rev1", `C:\logs\a.log`, "yo", nil)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := db.InsertDeployment("w", "rev2", "", "yo", map[string]string{"A": "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployment(oldID, "ok", "prev", "rev1"); err != nil {
		t.Fatal(err)
	}
	if st, ok := db.DeploymentStatus(newID); !ok || st != "running" {
		t.Fatalf("status running = %q %v", st, ok)
	}
	if st, ok := db.DeploymentStatus(oldID); !ok || st != "ok" {
		t.Fatalf("status ok = %q %v", st, ok)
	}
	if _, ok := db.DeploymentStatus(999999); ok {
		t.Fatal("id inexistente no debe existir")
	}
	// emvejecer la fila vieja
	if _, err := db.sql.Exec(`UPDATE deployments SET finished_at=? WHERE id=?`,
		time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339), oldID); err != nil {
		t.Fatal(err)
	}
	deps, err := db.ListDeployments("w", 10)
	if err != nil || len(deps) != 2 {
		t.Fatalf("deps = %+v %v", deps, err)
	}
	all, err := db.ListDeployments("", 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %+v %v", all, err)
	}
	// la fila running no se poda
	n, err := db.PruneDeployments(time.Now().Add(-24 * time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune = %d %v", n, err)
	}
	if _, err := db.ListDeployments("w", 10); err != nil {
		t.Fatal(err)
	}

	// log path: con ruta, sin ruta y inexistente
	app, lp, ok := db.DeploymentLogPath(newID)
	if ok || app != "" || lp != "" {
		t.Fatalf("log vacío no debe devolver ok: %q %q %v", app, lp, ok)
	}
	if _, _, ok := db.DeploymentLogPath(99999); ok {
		t.Fatal("id inexistente no debe devolver ok")
	}
	withLog, _ := db.InsertDeployment("w", "rev3", `C:\logs\b.log`, "yo", map[string]string{})
	app, lp, ok = db.DeploymentLogPath(withLog)
	if !ok || app != "w" || !strings.HasSuffix(lp, "b.log") {
		t.Fatalf("log path = %q %q %v", app, lp, ok)
	}

	// eventos + prune
	for _, k := range []string{"died", "died", "deploy"} {
		if err := db.InsertEvent(k, "w", "msg"); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := db.CountEvents("died"); err != nil || n != 2 {
		t.Fatalf("count died = %d %v", n, err)
	}
	if _, err := db.sql.Exec(`UPDATE events SET ts=?`, time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if n, err := db.PruneEvents(time.Now().Add(-24 * time.Hour)); err != nil || n != 3 {
		t.Fatalf("prune events = %d %v", n, err)
	}
}

func TestSessionsYUsuarios(t *testing.T) {
	db := newDB(t)
	if !ValidUserName("abc-_9") || ValidUserName("") || ValidUserName("ABC") || ValidUserName(strings.Repeat("a", 65)) {
		t.Fatal("ValidUserName mal")
	}
	if err := ValidateCredential("bad name", "123456789012"); err == nil {
		t.Fatal("nombre inválido debe fallar")
	}
	if err := ValidateCredential("ok", "corta"); err == nil {
		t.Fatal("password corta debe fallar")
	}
	if err := db.CreateUser("admin1", "password-larga-1", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser("admin1", "password-larga-1", RoleAdmin); err == nil {
		t.Fatal("usuario duplicado debe fallar")
	}
	if err := db.CreateUser("otro", "password-larga-1", "root"); err == nil {
		t.Fatal("rol inválido debe fallar")
	}
	if err := db.CreateUser("viewer1", "password-larga-1", RoleViewer); err != nil {
		t.Fatal(err)
	}
	users, err := db.ListUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("users = %+v %v", users, err)
	}

	if role, err := db.CheckPassword("admin1", "password-larga-1"); err != nil || role != RoleAdmin {
		t.Fatalf("check = %q %v", role, err)
	}
	if _, err := db.CheckPassword("admin1", "mala"); err == nil {
		t.Fatal("password mala debe fallar")
	}
	if _, err := db.CheckPassword("nadie", "x"); err == nil {
		t.Fatal("usuario inexistente debe fallar")
	}

	tok, err := db.NewSession("admin1", time.Hour)
	if err != nil || tok == "" {
		t.Fatalf("session = %q %v", tok, err)
	}
	if user, role, ok := db.SessionUser(tok); !ok || user != "admin1" || role != RoleAdmin {
		t.Fatalf("SessionUser = %q %q %v", user, role, ok)
	}
	if _, _, ok := db.SessionUser("token-basura"); ok {
		t.Fatal("token inválido no debe validar")
	}
	// sesión expirada: se crea con TTL negativo
	exp, err := db.NewSession("admin1", -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := db.SessionUser(exp); ok {
		t.Fatal("sesión expirada no debe validar")
	}
	// sesión de usuario borrado: rol no encontrado
	ghost, _ := db.NewSession("fantasma", time.Hour)
	if _, _, ok := db.SessionUser(ghost); ok {
		t.Fatal("sesión sin usuario no debe validar")
	}
	db.DeleteSession(ghost)
	db.DeleteSession("no-existe")

	if err := db.SetPassword("admin1", "corta"); err == nil {
		t.Fatal("password corta debe fallar")
	}
	if err := db.SetPassword("nadie", "password-larga-2"); err == nil {
		t.Fatal("usuario inexistente debe fallar")
	}
	if err := db.SetPassword("admin1", "password-larga-2"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := db.SessionUser(tok); ok {
		t.Fatal("SetPassword debe invalidar sesiones")
	}
	if err := db.DeleteUser("nadie"); err == nil {
		t.Fatal("borrar inexistente debe fallar")
	}
	if err := db.DeleteUser("admin1"); err == nil {
		t.Fatal("no se puede borrar el último admin")
	}
	if err := db.CreateUser("admin2", "password-larga-1", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser("admin1"); err != nil {
		t.Fatalf("borrar admin con otro admin: %v", err)
	}
}

func TestOpenCloseYErrores(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "archivo")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(file, "sub", "rkt.db")); err == nil {
		t.Fatal("Open bajo archivo debe fallar")
	}
	// archivo corrupto: PRAGMA/migración fallan
	bad := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(bad, []byte("no es sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bad); err == nil {
		t.Fatal("Open con DB corrupta debe fallar")
	}
	var nilDB *DB
	if err := nilDB.Close(); err != nil {
		t.Fatal("Close nil debe ser no-op")
	}
	db := newDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_ = db.Close() // idempotente
	// operaciones con DB cerrada cubren ramas de error
	if _, err := db.LoadInfra(); err == nil {
		t.Fatal("LoadInfra cerrada debe fallar")
	}
	if err := db.SaveInfra([]byte("{}")); err == nil {
		t.Fatal("SaveInfra cerrada debe fallar")
	}
	if _, err := db.ListRules(); err == nil {
		t.Fatal("ListRules cerrada debe fallar")
	}
	if _, err := db.LoadFleet(); err == nil {
		t.Fatal("LoadFleet cerrada debe fallar")
	}
	if _, err := db.ListUsers(); err == nil {
		t.Fatal("ListUsers cerrada debe fallar")
	}
	if _, err := db.ListDeployments("", 1); err == nil {
		t.Fatal("ListDeployments cerrada debe fallar")
	}
	if _, err := db.ListEvents(1, "", ""); err == nil {
		t.Fatal("ListEvents cerrada debe fallar")
	}
	if _, err := db.CountUsers(); err == nil {
		t.Fatal("CountUsers cerrada debe fallar")
	}
	if _, err := db.CountEvents("x"); err == nil {
		t.Fatal("CountEvents cerrada debe fallar")
	}
	if _, err := db.PruneDeployments(time.Now()); err == nil {
		t.Fatal("PruneDeployments cerrada debe fallar")
	}
	if _, err := db.PruneEvents(time.Now()); err == nil {
		t.Fatal("PruneEvents cerrada debe fallar")
	}
	if _, err := db.InsertDeployment("a", "b", "", "", nil); err == nil {
		t.Fatal("InsertDeployment cerrada debe fallar")
	}
	if err := db.FinishDeployment(1, "ok", "", ""); err == nil {
		t.Fatal("FinishDeployment cerrada debe fallar")
	}
	if err := db.InsertEvent("x", "y", "z"); err == nil {
		t.Fatal("InsertEvent cerrada debe fallar")
	}
	if err := db.SetSetting("k", "v"); err == nil {
		t.Fatal("SetSetting cerrada debe fallar")
	}
	if _, err := db.AddRoute(nil, ""); err == nil {
		t.Fatal("AddRoute cerrada debe fallar")
	}
	if err := db.UpsertApp(&config.App{Name: "x"}); err == nil {
		t.Fatal("UpsertApp cerrada debe fallar")
	}
	if err := db.ImportFleet(&config.Config{}); err == nil {
		t.Fatal("ImportFleet cerrada debe fallar")
	}
	if _, err := db.AddRule("a", "failed", 1, 60); err == nil {
		t.Fatal("AddRule cerrada debe fallar")
	}
	if err := db.DeleteRule(1); err == nil {
		t.Fatal("DeleteRule cerrada debe fallar")
	}
	if err := db.MarkFired(1, time.Now()); err == nil {
		t.Fatal("MarkFired cerrada debe fallar")
	}
	if _, err := db.NewSession("x", time.Hour); err == nil {
		t.Fatal("NewSession cerrada debe fallar")
	}
	if err := db.SetPassword("x", "password-larga-1"); err == nil {
		t.Fatal("SetPassword cerrada debe fallar")
	}
	if err := db.DeleteUser("x"); err == nil {
		t.Fatal("DeleteUser cerrada debe fallar")
	}
}
