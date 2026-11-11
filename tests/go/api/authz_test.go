package api

// Matriz authZ: cada endpoint × rol con el código esperado.
//
// Dos convenciones del código que el test fija:
//   - lookup primero: con ID/app inexistente el 404 llega antes que el
//     guard (mismo 404 para todos los roles; el guard se pisa con el
//     recurso existente).
//   - env/github/waf/fs/zones/suggest son operator+ aunque lean:
//     exponen superficie (repos, vars, IPs, rutas del disco).
//
// Sin token todo da 401 (misma vuelta).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/store"
)

type authzRow struct {
	method, path, body      string
	viewer, operator, admin int
}

func TestMatrizAuthZ(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	admTok := loginAs(t, h, db, "adm", store.RoleAdmin)

	// "ext" externa por el camino canónico (persiste en DB: sobrevive a
	// los mutateFleet de la matriz, que recargan la flota desde DB).
	extApp, _, extPort, err := cfgfile.CreateExternalApp(db, "ext", "ext.test", t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfgfile.AddApp(db, extApp, []string{"ext.test"}, extPort); err != nil {
		t.Fatal(err)
	}
	snap, err := cfgfile.Snapshot(db, d.Live.Get())
	if err != nil {
		t.Fatal(err)
	}
	d.Live.Swap(snap)

	rows := []authzRow{
		// lecturas abiertas a viewer
		{"GET", "/api/v1/status", "", 200, 200, 200},
		{"GET", "/api/v1/apps", "", 200, 200, 200},
		{"GET", "/api/v1/apps/w", "", 200, 200, 200},
		{"GET", "/api/v1/apps/w/usage", "", 200, 200, 200},
		{"GET", "/api/v1/apps/no-existe", "", 404, 404, 404},
		{"GET", "/api/v1/routes", "", 200, 200, 200},
		{"GET", "/api/v1/deployments", "", 200, 200, 200},
		{"GET", "/api/v1/deployments/99999", "", 404, 404, 404},
		{"GET", "/api/v1/deployments/99999/log", "", 404, 404, 404},
		{"GET", "/api/v1/events", "", 200, 200, 200},
		{"GET", "/api/v1/settings", "", 200, 200, 200},
		{"GET", "/api/v1/usage", "", 200, 200, 200},
		{"GET", "/api/v1/analytics", "", 200, 200, 200},
		{"GET", "/api/v1/observability", "", 200, 200, 200},
		{"GET", "/api/v1/server", "", 200, 200, 200},
		{"GET", "/api/v1/server/history", "", 200, 200, 200},
		{"GET", "/api/v1/server/processes", "", 200, 200, 200},
		{"GET", "/api/v1/server/doctor", "", 200, 200, 200},
		{"GET", "/api/v1/proxy", "", 200, 200, 200},
		{"GET", "/api/v1/proxy/capabilities", "", 200, 200, 200},
		{"GET", "/api/v1/cron", "", 200, 200, 200},
		{"GET", "/api/v1/alerts", "", 200, 200, 200},
		{"GET", "/api/v1/domains/no-existe.test", "", 404, 404, 404},
		{"GET", "/api/v1/domains/hosts", "", 200, 200, 200},
		{"GET", "/api/v1/logs?app=w", "", 404, 404, 404}, // sin archivo aún
		// lecturas operator+ (superficie sensible)
		{"GET", "/api/v1/audit", "", 403, 403, 200},
		{"GET", "/api/v1/github", "", 403, 200, 200},
		{"GET", "/api/v1/github/repos", "", 403, 200, 200},
		{"GET", "/api/v1/alert-channels", "", 403, 200, 200},
		{"GET", "/api/v1/domains/zones", "", 403, 200, 200},
		{"GET", "/api/v1/domains/suggest?name=tienda", "", 403, 200, 200},
		{"GET", "/api/v1/env", "", 403, 200, 200},
		{"GET", "/api/v1/env/w", "", 403, 404, 404}, // sin ruta por app
		{"GET", "/api/v1/fs?path=/", "", 403, 200, 200},
		{"GET", "/api/v1/waf", "", 403, 200, 200},
		{"GET", "/api/v1/detect", "", 403, 400, 400},
		{"GET", "/api/v1/apps/w/diff", "", 403, 400, 400},
		// mutaciones sobre inexistente: 404 antes que el guard
		{"PUT", "/api/v1/apps/no-existe", `{}`, 404, 404, 404},
		{"DELETE", "/api/v1/apps/no-existe", "", 404, 404, 404},
		{"POST", "/api/v1/apps/no-existe?action=deploy", "", 404, 404, 404},
		{"POST", "/api/v1/apps/no-existe?action=rollback", "", 404, 404, 404},
		// mutaciones sobre existente: el guard sí muerde
		{"PUT", "/api/v1/apps/w", `{}`, 403, 200, 200},
		{"POST", "/api/v1/apps/w/deploy", "", 403, 202, 202}, // build local: 202 + id
		{"POST", "/api/v1/apps/w/rollback", "", 403, 200, 200},
		{"POST", "/api/v1/apps/w/build", "", 403, 200, 200},
		{"POST", "/api/v1/apps/w/restart", "", 403, 200, 200},
		{"POST", "/api/v1/apps/w/start", "", 403, 200, 200},
		{"POST", "/api/v1/apps/w/stop", "", 403, 200, 200},
		// externa: 409 antes que el guard
		{"POST", "/api/v1/apps/ext/deploy", "", 409, 409, 409},
		{"POST", "/api/v1/apps/ext/rollback", "", 409, 409, 409},
		// resto de mutaciones
		{"POST", "/api/v1/apps", `{}`, 403, 400, 400},
		{"POST", "/api/v1/routes", `{}`, 403, 400, 400},
		{"DELETE", "/api/v1/routes?host=no-existe.test", "", 403, 404, 404},
		{"PUT", "/api/v1/settings", `{}`, 403, 400, 400},
		{"POST", "/api/v1/alerts", `{}`, 403, 400, 400},
		{"DELETE", "/api/v1/alerts/999", "", 403, 400, 400},
		{"POST", "/api/v1/alert-channels", `{}`, 403, 400, 400},
		{"PUT", "/api/v1/waf", `{}`, 403, 200, 200},
		{"DELETE", "/api/v1/waf/bans?ip=9.9.9.9", "", 403, 404, 404},
		{"POST", "/api/v1/deployments/7/retry", "", 403, 200, 200},
		{"POST", "/api/v1/deployments/99999/retry", "", 403, 400, 400},
		{"POST", "/api/v1/deployments/99999/cancel", "", 403, 400, 400},
		{"PUT", "/api/v1/cron/w/j1", `{"schedule":"* * * * *","type":"command","command":"echo","timeout":"30s"}`, 403, 200, 200},
		{"DELETE", "/api/v1/cron/w/j1", "", 403, 200, 400}, // el operator ya lo borró
		{"POST", "/api/v1/cron", `{}`, 405, 405, 405},
		{"PUT", "/api/v1/env/global", `{"set":{"G":"1"}}`, 403, 200, 200},
		{"POST", "/api/v1/env/w", `{"A":"b"}`, 403, 404, 404},
		// usuarios y tokens: solo admin
		{"GET", "/api/v1/users", "", 403, 403, 200},
		{"GET", "/api/v1/users/vis/sessions", "", 200, 403, 200},
		{"POST", "/api/v1/users", `{}`, 403, 403, 400},
		{"PUT", "/api/v1/users/fantasma", `{"role":"viewer"}`, 403, 403, 400},
		{"DELETE", "/api/v1/users/fantasma", "", 403, 403, 400},
		{"GET", "/api/v1/tokens", "", 403, 403, 200},
		{"POST", "/api/v1/tokens", `{}`, 403, 403, 400},
		// al final: borra w (ya verificado arriba)
		{"DELETE", "/api/v1/apps/w", "", 403, 200, 404}, // el operator ya la borró
	}

	for _, r := range rows {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			try := func(tok string) int {
				var req *http.Request
				if r.body != "" {
					req = httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
				} else {
					req = httptest.NewRequest(r.method, r.path, nil)
				}
				if tok != "" {
					req.Header.Set("Authorization", "Bearer "+tok)
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				return rec.Code
			}
			if got := try(""); got != http.StatusUnauthorized {
				t.Fatalf("sin token: %d, quiere 401", got)
			}
			if got := try(visTok); got != r.viewer {
				t.Fatalf("viewer: %d, quiere %d", got, r.viewer)
			}
			if got := try(opTok); got != r.operator {
				t.Fatalf("operator: %d, quiere %d", got, r.operator)
			}
			if got := try(admTok); got != r.admin {
				t.Fatalf("admin: %d, quiere %d", got, r.admin)
			}
		})
	}
}
