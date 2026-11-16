// Package authz: único verificador de autorización y tabla única de roles.
//
// Los transportes (API, MCP-HTTP, MCP-stdio, healthz) resuelven credencial
// a rol cada uno a su manera; la DECISIÓN siempre pasa por Check con la
// misma tabla op→rol mínimo. Sin entrada en la tabla, un op exige admin
// (fail-closed). Roles: viewer < operator < admin; local (bootstrap
// loopback con control.token) equivale a operator, nunca a admin.
package authz

import (
	"net/http"
	"net/url"
	"strings"
)

// Role rol de un actor autenticado.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
	// RoleLocal bootstrap loopback (control.token leído de disco).
	// Autoriza como operator; las ops admin le están vedadas.
	RoleLocal Role = "local"
)

// Op operación canónica. Un op por capacidad, no por transporte:
// la misma op cubre el endpoint HTTP, la tool MCP y la acción healthz.
type Op string

const (
	OpStatusRead      Op = "status.read"
	OpLogsRead        Op = "logs.read"
	OpDeploymentsRead Op = "deployments.read"
	OpEventsRead      Op = "events.read"
	OpMetricsRead     Op = "metrics.read"
	OpAppsRead        Op = "apps.read"
	OpRoutesRead      Op = "routes.read"
	OpEnvKeysRead     Op = "env.keys.read"
	OpDomainsRead     Op = "domains.read"
	OpZonesRead       Op = "zones.read"
	OpDetectRead      Op = "detect.read"
	OpAlertsRead      Op = "alerts.read"
	OpCronRead        Op = "cron.read"
	OpAuditRead       Op = "audit.read"

	OpAppsWrite     Op = "apps.write"
	OpDeployRun     Op = "deploy.run"
	OpRoutesWrite   Op = "routes.write"
	OpEnvWrite      Op = "env.write"
	OpEnvRead       Op = "env.read"
	OpSettingsWrite Op = "settings.write"
	OpSettingsRead  Op = "settings.read"
	OpCronWrite     Op = "cron.write"
	OpAlertsWrite   Op = "alerts.write"
	OpWAFWrite      Op = "waf.write"
	OpWAFRead       Op = "waf.read"
	OpHostsWrite    Op = "hosts.write"
	OpDiscoverRun   Op = "discover.run"
	OpGitHubWrite   Op = "github.write"
	OpFsRead        Op = "fs.read"
	OpConfigExport  Op = "config.export"
	OpPgRead        Op = "pg.read"
	OpPgWrite       Op = "pg.write"

	OpUsersAdmin   Op = "users.admin"
	OpTokensAdmin  Op = "tokens.admin"
	OpConfigImport Op = "config.import"
	OpPrune        Op = "prune"
)

// opMinRole tabla única operación -> rol mínimo.
var opMinRole = map[Op]Role{
	OpStatusRead:      RoleViewer,
	OpLogsRead:        RoleViewer,
	OpDeploymentsRead: RoleViewer,
	OpEventsRead:      RoleViewer,
	OpMetricsRead:     RoleViewer,
	OpAppsRead:        RoleViewer,
	OpRoutesRead:      RoleViewer,
	OpEnvKeysRead:     RoleViewer,
	OpDomainsRead:     RoleViewer,
	OpZonesRead:       RoleViewer,
	OpDetectRead:      RoleViewer,
	OpAlertsRead:      RoleViewer,
	OpCronRead:        RoleViewer,

	OpAppsWrite:     RoleOperator,
	OpDeployRun:     RoleOperator,
	OpRoutesWrite:   RoleOperator,
	OpEnvWrite:      RoleOperator,
	OpSettingsWrite: RoleOperator,
	OpCronWrite:     RoleOperator,
	OpAlertsWrite:   RoleOperator,
	OpWAFWrite:      RoleOperator,
	OpDiscoverRun:   RoleOperator,
	OpGitHubWrite:   RoleOperator,
	OpFsRead:        RoleOperator,
	OpConfigExport:  RoleOperator,
	OpPgRead:        RoleViewer,
	OpPgWrite:       RoleOperator,
	OpEnvRead:       RoleOperator,
	OpSettingsRead:  RoleOperator,
	OpWAFRead:       RoleOperator,

	OpHostsWrite:   RoleAdmin,
	OpUsersAdmin:   RoleAdmin,
	OpTokensAdmin:  RoleAdmin,
	OpConfigImport: RoleAdmin,
	OpPrune:        RoleAdmin,
	OpAuditRead:    RoleAdmin,
}

// MinRole rol mínimo para un op; desconocido = admin (fail-closed).
func MinRole(op Op) Role {
	if r, ok := opMinRole[op]; ok {
		return r
	}
	return RoleAdmin
}

// level orden total: local == operator (bootstrap, nunca admin).
func level(r Role) int {
	switch r {
	case RoleAdmin:
		return 3
	case RoleOperator, RoleLocal:
		return 2
	case RoleViewer:
		return 1
	}
	return 0
}

// AtLeast true si have cubre want en la jerarquía.
func AtLeast(have, want Role) bool {
	return level(have) >= level(want) && level(want) > 0 && level(have) > 0
}

// Check autoriza rol para op; local jamas pasa una op admin.
func Check(role Role, op Op) bool {
	want := MinRole(op)
	if want == RoleAdmin && role == RoleLocal {
		return false
	}
	return level(role) >= level(want)
}

// SameOrigin compara Origin con Host (exacto host+puerto). dev relaja
// loopback<->loopback (proxy Vite) igual que la API.
func SameOrigin(originHost, reqHost string, dev bool) bool {
	if strings.EqualFold(originHost, reqHost) {
		return true
	}
	if dev {
		return IsLoopbackHost(HostOnly(originHost)) && IsLoopbackHost(HostOnly(reqHost))
	}
	return false
}

// HostOnly quita el puerto.
func HostOnly(h string) string {
	if i := strings.LastIndex(h, ":"); i >= 0 {
		if strings.HasSuffix(h, "]") {
			return h
		}
		return h[:i]
	}
	return h
}

// IsLoopbackHost loopback con o sin corchetes.
func IsLoopbackHost(h string) bool {
	h = strings.ToLower(strings.Trim(h, "[]"))
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// OriginOf extrae el host del Origin o "" si falta/inválido.
func OriginOf(r *http.Request) string {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Host
}
