package authz

import (
	"testing"

	"rkt/internal/store"
)

// Los strings de rol son el contrato con store (sesiones y tokens los
// guardan así). Si cambian allá, esto avisa antes que el gate.
func TestRolesSincronizados(t *testing.T) {
	if string(RoleViewer) != store.RoleViewer || string(RoleOperator) != store.RoleOperator || string(RoleAdmin) != store.RoleAdmin {
		t.Fatal("authz desincronizado de store")
	}
}

// Jerarquía y bootstrap local: la tabla única decide, no el transporte.
func TestJerarquia(t *testing.T) {
	if !AtLeast(RoleAdmin, RoleViewer) || !AtLeast(RoleOperator, RoleViewer) {
		t.Fatal("admin/operator deben cubrir viewer")
	}
	if AtLeast(RoleViewer, RoleOperator) || AtLeast(RoleViewer, RoleAdmin) {
		t.Fatal("viewer no sube")
	}
	if !Check(RoleLocal, OpAppsWrite) || !Check(RoleLocal, OpDeployRun) {
		t.Fatal("local equivale a operator")
	}
	if Check(RoleLocal, OpUsersAdmin) || Check(RoleLocal, OpHostsWrite) || Check(RoleLocal, OpPrune) {
		t.Fatal("local jamás pasa admin")
	}
	if MinRole(Op("inexistente")) != RoleAdmin {
		t.Fatal("op desconocido debe ser admin (fail-closed)")
	}
}

// Contrato unificado: rol mínimo por tool (API, MCP-HTTP y MCP-stdio
// comparten esta tabla vía Op). Cambiar un rol aquí cambia los tres.
func TestRolesPorTool(t *testing.T) {
	viewers := []string{"status", "logs", "deployments_list", "events_list", "version",
		"metrics_now", "traffic_rate", "proxy_map", "backends_probe", "usage_summary",
		"zones_list", "alerts_list", "discover_status", "app_get", "app_env_keys",
		"deployment_get", "deployment_logs", "log_search", "tail_follow", "git_status",
		"diff_rev", "disk_usage", "detect_plan", "domains_suggest", "verify_domain",
		"watch_status", "watch_events", "watch_deploy", "deploy_wait", "wait_healthy",
		"env_global_list", "process_stats", "app_usage", "apps_list", "routes_list"}
	operators := []string{"deploy", "retry", "rollback", "restart", "stop", "start",
		"build", "apps_add", "apps_remove", "app_update", "env_set", "env_delete",
		"env_import", "env_global_set", "env_global_delete", "routes_add",
		"routes_update", "routes_remove", "alerts_add", "alerts_update",
		"alerts_remove", "alerts_test", "config_export", "discover", "deploy_check"}
	admins := []string{"users_add", "users_passwd", "users_remove", "config_import",
		"prune", "hosts_add", "hosts_remove"}
	for _, tool := range viewers {
		if got := MinRole(mustOp(t, tool)); got != RoleViewer {
			t.Fatalf("%s: %s, quiere viewer", tool, got)
		}
	}
	for _, tool := range operators {
		if got := MinRole(mustOp(t, tool)); got != RoleOperator {
			t.Fatalf("%s: %s, quiere operator", tool, got)
		}
	}
	for _, tool := range admins {
		if got := MinRole(mustOp(t, tool)); got != RoleAdmin {
			t.Fatalf("%s: %s, quiere admin", tool, got)
		}
	}
}

func mustOp(t *testing.T, tool string) Op {
	t.Helper()
	op, ok := ToolOp(tool)
	if !ok {
		t.Fatalf("tool sin op: %s", tool)
	}
	return op
}

// SameOrigin estricto salvo loopback en dev (proxy Vite).
func TestSameOrigin(t *testing.T) {
	if !SameOrigin("a.test:8090", "a.test:8090", false) {
		t.Fatal("mismo host+puerto")
	}
	if SameOrigin("evil.test", "a.test:8090", false) {
		t.Fatal("otro host pasa")
	}
	if SameOrigin("localhost:5173", "127.0.0.1:8090", false) {
		t.Fatal("puertos distintos pasan en prod")
	}
	if !SameOrigin("localhost:5173", "127.0.0.1:8090", true) {
		t.Fatal("dev relaja loopback")
	}
}
