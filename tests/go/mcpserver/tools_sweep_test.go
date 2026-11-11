package mcpserver

// Barrido por herramienta: cada do* con args inválidos debe fallar
// (no pánico, no éxito mudo) y los flujos con estado hacen round-trip
// real sobre DB temporal. Sin red ni procesos: lo que necesita servicio
// responde contra el httptest de testServer.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func mustErr(t *testing.T, name string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: esperaba error", name)
	}
}

func mustOK(t *testing.T, name, out string, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return out
}

func parseRuleID(t *testing.T, out string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(out, "regla #%d", &id); err != nil || id <= 0 {
		t.Fatalf("id de regla en %q: %v", out, err)
	}
	return id
}

func TestHerramientasValidacion(t *testing.T) {
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	bad := []struct {
		name string
		fn   func() (string, error)
	}{
		{"discover sin github", s.doDiscover},
		{"build sin app", func() (string, error) { return s.doBuild("") }},
		{"deploy sin app", func() (string, error) { return s.doDeploy("") }},
		{"retry sin id", func() (string, error) { return s.doRetry(0) }},
		{"rollback sin app", func() (string, error) { return s.doRollback("") }},
		{"restart sin app", func() (string, error) { return s.doRestart("") }},
		{"stop sin app", func() (string, error) { return s.doAppSetDisabled("", true) }},
		{"app_update sin app", func() (string, error) { return s.doAppUpdate(appUpdateArg{}) }},
		{"apps_remove sin app", func() (string, error) { return s.doAppsRemove(removeAppArg{}) }},
		{"apps_add sin repo", func() (string, error) { return s.doAppsAdd(appsAddArg{}) }},
		{"app_get fantasma", func() (string, error) { return s.doAppGet("nope") }},
		{"app_env_keys fantasma", func() (string, error) { return s.doAppEnvKeys("nope") }},
		{"env_set sin app", func() (string, error) { return s.doEnvSet("", "K", "v") }},
		{"env_delete fantasma", func() (string, error) { return s.doEnvDelete(envDelArg{App: "nope", Key: "K"}) }},
		{"env_global_delete sin clave", func() (string, error) { return s.doEnvGlobalDelete(envGlobalArg{}) }},
		{"env_import roto", func() (string, error) { return s.doEnvImport(envImportArg{Raw: "=mal="}) }},
		{"routes_add sin hosts", func() (string, error) { return s.doRoutesAdd(routesAddArg{}) }},
		{"routes_update fantasma", func() (string, error) {
			return s.doRoutesUpdate(routesUpdateArg{Host: "nope.test", Target: "http://127.0.0.1:9"})
		}},
		{"routes_remove vacío", func() (string, error) { return s.doRoutesRemove(hostArg{}) }},
		{"hosts_add vacío", func() (string, error) { return s.doHostsAdd("") }},
		{"hosts_remove vacío", func() (string, error) { return s.doHostsRemove("") }},
		{"users_add corto", func() (string, error) { return s.doUsersAdd(userAddArg{Name: "u", Password: "corto"}) }},
		{"users_remove fantasma", func() (string, error) { return s.doUsersRemove(userArg{Name: "fantasma"}) }},
		{"users_passwd fantasma", func() (string, error) {
			return s.doUsersPasswd(userPassArg{Name: "fantasma", Password: "secreto-largo-123"})
		}},
		{"settings_set desconocida", func() (string, error) { return s.doSettingsSet(settingsSetArg{Key: "noexiste", Value: "1"}) }},
		{"config_import roto", func() (string, error) { return s.doConfigImport(importArg{JSON: "{roto", Confirm: "replace"}) }},
		{"config_import sin confirm", func() (string, error) { return s.doConfigImport(importArg{JSON: "{}"}) }},
		{"prune sin confirm", func() (string, error) { return s.doPrune(pruneArg{}) }},
		{"alerts_remove fantasma", func() (string, error) { return s.doAlertsRemove(99999) }},
		{"alerts_test fantasma", func() (string, error) { return s.doAlertsTest(99999) }},
		{"alerts_update fantasma", func() (string, error) { return s.doAlertsUpdate(alertUpdateArg{ID: 99999}) }},
		{"deployment_get cero", func() (string, error) { return s.doDeploymentGet(0) }},
		{"deployment_get fantasma", func() (string, error) { return s.doDeploymentGet(99999) }},
		{"deployment_logs fantasma", func() (string, error) { return s.doDeploymentLogs("nope", 5) }},
		{"wait_healthy sin app", func() (string, error) { return s.doWaitHealthy("", 1) }},
		{"verify_domain vacío", func() (string, error) { return s.doVerifyDomain("") }},
		{"disk_usage fantasma", func() (string, error) { return s.doDiskUsage("nope") }},
		{"git_status fantasma", func() (string, error) { return s.doGitStatus("nope") }},
		{"diff_rev fantasma", func() (string, error) { return s.doDiffRev("nope") }},
		{"process_stats fantasma", func() (string, error) { return s.doProcessStats("nope") }},
		{"log_search regex rota", func() (string, error) { return s.doLogSearch(logSearchArg{Pattern: "["}) }},
		{"tail_follow regex rota", func() (string, error) { return s.doTailFollow(tailFollowArg{Pattern: "["}) }},
		{"detect_plan vacío", func() (string, error) { return s.doDetectPlan("") }},
		{"detect_plan sin proyecto", func() (string, error) { return s.doDetectPlan(t.TempDir()) }},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.fn()
			mustErr(t, c.name, err)
		})
	}
}

func TestHerramientasRoundTrip(t *testing.T) {
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	var out string
	var err error

	// usuarios: alta, cambio de clave, baja.
	out, err = s.doUsersAdd(userAddArg{Name: "u1", Password: "secreto-largo-123"})
	mustOK(t, "users_add", out, err)
	out, err = s.doUsersAdd(userAddArg{Name: "u2", Password: "otro-secreto-456", Role: "operator"})
	mustOK(t, "users_add", out, err)
	out, err = s.doUsersPasswd(userPassArg{Name: "u1", Password: "nuevo-secreto-789"})
	mustOK(t, "users_passwd", out, err)
	out, err = s.doUsersRemove(userArg{Name: "u1"})
	mustOK(t, "users_remove", out, err)
	out, err = s.doUsersRemove(userArg{Name: "u2"})
	mustOK(t, "users_remove", out, err)

	// rutas: alta, lista, cambio de target, baja.
	out, err = s.doRoutesAdd(routesAddArg{Hosts: []string{"b.test"}, Target: "http://127.0.0.1:9"})
	mustOK(t, "routes_add", out, err)
	out, err = s.doRoutesList()
	if mustOK(t, "routes_list", out, err); !strings.Contains(out, "b.test") {
		t.Fatalf("routes_list sin b.test: %s", out)
	}
	out, err = s.doRoutesUpdate(routesUpdateArg{Host: "b.test", Target: "http://127.0.0.1:10"})
	mustOK(t, "routes_update", out, err)
	out, err = s.doRoutesRemove(hostArg{Host: "b.test"})
	mustOK(t, "routes_remove", out, err)
	out, err = s.doRoutesList()
	if mustOK(t, "routes_list", out, err); strings.Contains(out, "b.test") {
		t.Fatalf("routes_remove no quitó b.test: %s", out)
	}

	// apps: alta local (estática, sin red), get, update, stop/start, env, baja.
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<h1>x</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = s.doAppsAdd(appsAddArg{Name: "demo1", Repo: staticDir})
	if added := mustOK(t, "apps_add", out, err); !strings.Contains(added, "demo1") {
		t.Fatalf("apps_add: %s", added)
	}
	out, err = s.doAppsList()
	if mustOK(t, "apps_list", out, err); !strings.Contains(out, "demo1") {
		t.Fatalf("apps_list sin demo1: %s", out)
	}
	out, err = s.doAppGet("demo1")
	mustOK(t, "app_get", out, err)
	hp := "/salud"
	out, err = s.doAppUpdate(appUpdateArg{App: "demo1", HealthPath: &hp})
	mustOK(t, "app_update", out, err)
	out, err = s.doAppSetDisabled("demo1", true)
	mustOK(t, "stop", out, err)
	out, err = s.doAppSetDisabled("demo1", false)
	mustOK(t, "start", out, err)
	out, err = s.doEnvSet("demo1", "K", "v")
	mustOK(t, "env_set", out, err)
	out, err = s.doAppEnvKeys("demo1")
	if mustOK(t, "app_env_keys", out, err); !strings.Contains(out, "K") {
		t.Fatalf("app_env_keys sin K: %s", out)
	}
	out, err = s.doEnvDelete(envDelArg{App: "demo1", Key: "K"})
	mustOK(t, "env_delete", out, err)
	out, err = s.doDetectPlan(staticDir)
	if plan := mustOK(t, "detect_plan", out, err); !strings.Contains(plan, "static") {
		t.Fatalf("detect_plan: %s", plan)
	}
	// verify_domain nunca falla: reporta dentro del JSON (diagnóstico).
	out, err = s.doVerifyDomain("a.test")
	if out2 := mustOK(t, "verify_domain", out, err); !strings.Contains(out2, "http_error") && !strings.Contains(out2, "dns_ok") {
		t.Fatalf("verify_domain: %s", out2)
	}
	// git_status en no-repo responde vacío, sin error (diagnóstico leniente).
	out, err = s.doGitStatus("demo1")
	if mustOK(t, "git_status", out, err); !strings.Contains(out, "demo1") {
		t.Fatalf("git_status: %s", out)
	}
	if _, err := s.doDiffRev("demo1"); err == nil {
		t.Fatal("diff_rev no-repo: esperaba error")
	}
	out, err = s.doDiskUsage("demo1")
	mustOK(t, "disk_usage", out, err)
	out, err = s.doAppsRemove(removeAppArg{App: "demo1"})
	mustOK(t, "apps_remove", out, err)

	// globales: set, lista, import, delete.
	out, err = s.doEnvGlobalSet(envGlobalArg{Key: "GLOB", Value: "1"})
	mustOK(t, "env_global_set", out, err)
	out, err = s.doEnvGlobalList(envGlobalListArg{})
	if mustOK(t, "env_global_list", out, err); !strings.Contains(out, "GLOB") {
		t.Fatalf("env_global_list sin GLOB: %s", out)
	}
	out, err = s.doEnvImport(envImportArg{Raw: "IMP=2\n"})
	mustOK(t, "env_import", out, err)
	out, err = s.doEnvGlobalDelete(envGlobalArg{Key: "GLOB"})
	mustOK(t, "env_global_delete", out, err)
	out, err = s.doEnvGlobalDelete(envGlobalArg{Key: "IMP"})
	mustOK(t, "env_global_delete", out, err)

	// settings + config export/import.
	out, err = s.doSettingsSet(settingsSetArg{Key: "base_domain", Value: "ej.test"})
	mustOK(t, "settings_set", out, err)
	out, err = s.doSettingsList()
	if mustOK(t, "settings_list", out, err); !strings.Contains(out, "base_domain") {
		t.Fatalf("settings_list: %s", out)
	}
	out, err = s.doConfigExport(exportArg{})
	exp := mustOK(t, "config_export", out, err)
	if !strings.Contains(exp, "ej.test") {
		t.Fatalf("config_export sin base_domain: %s", exp)
	}
	out, err = s.doConfigImport(importArg{JSON: exp, Confirm: "replace"})
	mustOK(t, "config_import", out, err)

	// alertas: alta (defaults), describe sin ejecutar, update, baja.
	out, err = s.doAlertsAdd(alertAddArg{})
	ruleID := parseRuleID(t, mustOK(t, "alerts_add", out, err))
	out, err = s.doAlertsTest(ruleID)
	mustOK(t, "alerts_test", out, err)
	th := 5.0
	out, err = s.doAlertsUpdate(alertUpdateArg{ID: ruleID, Threshold: &th})
	mustOK(t, "alerts_update", out, err)
	out, err = s.doAlertsRemove(ruleID)
	mustOK(t, "alerts_remove", out, err)

	// prune con confirmación (vacío: 0, sin error).
	out, err = s.doPrune(pruneArg{Confirm: "prune"})
	mustOK(t, "prune", out, err)

	// deployment_get con fila real.
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	depID, err := db.InsertDeployment("w", "abc1234", "", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployment(depID, "ok", "", "abc1234"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, err = s.doDeploymentGet(depID)
	if mustOK(t, "deployment_get", out, err); !strings.Contains(out, "abc1234") {
		t.Fatalf("deployment_get: %s", out)
	}

	// logs sembrados aquí (testServer no trae logs).
	logsDir := filepath.Join(filepath.Dir(s.cfgPath), "logs")
	if err := os.MkdirAll(filepath.Join(logsDir, "apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "rkt.log"), []byte("l1\nl2\nl3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = s.doLogSearch(logSearchArg{Pattern: "l[12]"})
	if mustOK(t, "log_search", out, err); !strings.Contains(out, "l1") {
		t.Fatalf("log_search: %s", out)
	}
	out, err = s.doTailFollow(tailFollowArg{TimeoutS: 1})
	if mustOK(t, "tail_follow", out, err); !strings.Contains(out, "timeout") {
		t.Fatalf("tail_follow: %s", out)
	}
	out, err = s.doLogs(5, "")
	if mustOK(t, "logs operativo", out, err); !strings.Contains(out, "l1") {
		t.Fatalf("logs: %s", out)
	}

	// dominios + zonas + proxy + uso + métricas (la versión es build.Version directo).
	out, err = s.doDomainsSuggest("tienda")
	mustOK(t, "domains_suggest", out, err)
	out, err = s.doZonesList()
	mustOK(t, "zones_list", out, err)
	out, err = s.doProxyMap()
	mustOK(t, "proxy_map", out, err)
	out, err = s.doBackendsProbe()
	mustOK(t, "backends_probe", out, err)
	out, err = s.doUsageSummary()
	mustOK(t, "usage_summary", out, err)
	out, err = s.doAlertsList()
	mustOK(t, "alerts_list", out, err)
	out, err = s.doDiscoverStatus()
	mustOK(t, "discover_status", out, err)
	out, err = s.doMetricsNow()
	mustOK(t, "metrics_now", out, err)
	out, err = s.doTrafficRate(trafficArg{Seconds: 5})
	mustOK(t, "traffic_rate", out, err)

	// long-polls con tope 1s: terminan en timeout, no se cuelgan.
	for _, c := range []struct {
		name string
		fn   func() (string, error)
	}{
		{"watch_status", func() (string, error) { return s.doWatchStatus(watchStatusArg{TimeoutS: 1}) }},
		{"watch_events", func() (string, error) { return s.doWatchEvents(watchEventsArg{TimeoutS: 1}) }},
		{"watch_deploy", func() (string, error) { return s.doWatchDeployment(watchDeployArg{TimeoutS: 1}) }},
	} {
		out, err := c.fn()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(out, "timeout") {
			t.Fatalf("%s sin timeout: %s", c.name, out)
		}
	}
	// deploy_wait sin nada que esperar: error rápido, no cuelgue.
	if _, err := s.doDeployWait(deployWaitArg{App: "nope", TimeoutS: 1}); err == nil {
		t.Fatal("deploy_wait: esperaba error")
	}
	if _, err := s.doWaitHealthy("nope", 1); err == nil {
		t.Fatal("wait_healthy fantasma: esperaba error")
	}
	if _, err := s.doProcessStats("nope"); err == nil {
		t.Fatal("process_stats fantasma: esperaba error")
	}
}
