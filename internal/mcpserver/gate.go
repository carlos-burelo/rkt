package mcpserver

// Puerta MCP por herramienta: espejo del RBAC de la API (viewer lee,
// operator muta, admin gestiona usuarios/imports y el sistema).
// Sin entrada en la tabla la herramienta exige admin (fail-closed).
// Las mutaciones se auditan; las lecturas quedan solo en access.log.

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

// toolMinRole herramienta -> rol mínimo.
var toolMinRole = map[string]string{
	// lectura
	"status":           store.RoleViewer,
	"logs":             store.RoleViewer,
	"deployments_list": store.RoleViewer,
	"events_list":      store.RoleViewer,
	"version":          store.RoleViewer,
	"metrics_now":      store.RoleViewer,
	"traffic_rate":     store.RoleViewer,
	"proxy_map":        store.RoleViewer,
	"backends_probe":   store.RoleViewer,
	"usage_summary":    store.RoleViewer,
	"zones_list":       store.RoleViewer,
	"alerts_list":      store.RoleViewer,
	"discover_status":  store.RoleViewer,
	"app_get":          store.RoleViewer,
	"app_env_keys":     store.RoleViewer,
	"deployment_get":   store.RoleViewer,
	"deployment_logs":  store.RoleViewer,
	"log_search":       store.RoleViewer,
	"tail_follow":      store.RoleViewer,
	"git_status":       store.RoleViewer,
	"diff_rev":         store.RoleViewer,
	"disk_usage":       store.RoleViewer,
	"detect_plan":      store.RoleViewer,
	"domains_suggest":  store.RoleViewer,
	"verify_domain":    store.RoleViewer,
	"watch_status":     store.RoleViewer,
	"watch_events":     store.RoleViewer,
	"watch_deploy":     store.RoleViewer,
	"deploy_wait":      store.RoleViewer,
	"wait_healthy":     store.RoleViewer,
	"env_global_list":  store.RoleViewer,
	"process_stats":    store.RoleViewer,
	"app_usage":        store.RoleViewer,
	"apps_list":        store.RoleViewer,
	"routes_list":      store.RoleViewer,
	// mutación
	"deploy":            store.RoleOperator,
	"retry":             store.RoleOperator,
	"rollback":          store.RoleOperator,
	"restart":           store.RoleOperator,
	"stop":              store.RoleOperator,
	"start":             store.RoleOperator,
	"build":             store.RoleOperator,
	"apps_add":          store.RoleOperator,
	"apps_remove":       store.RoleOperator,
	"app_update":        store.RoleOperator,
	"env_set":           store.RoleOperator,
	"env_delete":        store.RoleOperator,
	"env_import":        store.RoleOperator,
	"env_global_set":    store.RoleOperator,
	"env_global_delete": store.RoleOperator,
	"routes_add":        store.RoleOperator,
	"routes_update":     store.RoleOperator,
	"routes_remove":     store.RoleOperator,
	"alerts_add":        store.RoleOperator,
	"alerts_update":     store.RoleOperator,
	"alerts_remove":     store.RoleOperator,
	"alerts_test":       store.RoleOperator,
	"config_export":     store.RoleOperator,
	"discover":          store.RoleOperator,
	"deploy_check":      store.RoleOperator,
	// administración y sistema
	"users_add":     store.RoleAdmin,
	"users_passwd":  store.RoleAdmin,
	"users_remove":  store.RoleAdmin,
	"config_import": store.RoleAdmin,
	"prune":         store.RoleAdmin,
	"hosts_add":     store.RoleAdmin,
	"hosts_remove":  store.RoleAdmin,
}

// minRoleFor rol mínimo; desconocida = admin (fail-closed).
func minRoleFor(tool string) string {
	if r, ok := toolMinRole[tool]; ok {
		return r
	}
	return store.RoleAdmin
}

func roleAtLeast(have, want string) bool {
	level := map[string]int{store.RoleViewer: 0, store.RoleOperator: 1, store.RoleAdmin: 2}
	h, okH := level[have]
	w, okW := level[want]
	return okH && okW && h >= w
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// tokenRole resuelve (rol, actor) del Bearer; vacío si no hay o no vale.
// Abre la DB por llamada, igual que cada handler (s.load).
func (s *Server) tokenRole(r *http.Request) (string, string) {
	tok := bearerToken(r)
	if tok == "" {
		return "", ""
	}
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		return "", ""
	}
	defer db.Close()
	name, role, ok := db.TokenUser(tok)
	if !ok {
		return "", ""
	}
	return role, name
}

type rpcCall struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name string `json:"name"`
	} `json:"params"`
}

// toolCallName extrae (herramienta, id) de un tools/call y restaura el
// body para el handler real. ok=false si no es un tools/call simple
// (listas, notificaciones, batches: pasan sin filtrar).
func toolCallName(r *http.Request) (string, any, bool) {
	if r.Method != http.MethodPost || r.Body == nil {
		return "", nil, false
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return "", nil, false
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	var c rpcCall
	if err := json.Unmarshal(b, &c); err != nil || c.Method != "tools/call" || c.Params.Name == "" {
		return "", nil, false
	}
	return c.Params.Name, c.ID, true
}

func writeRPCError(w http.ResponseWriter, id any, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": -32600, "message": msg},
	})
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// gateHTTP exige el rol de cada herramienta y audita las mutaciones.
// Las lecturas quedan en access.log; sin entrada en la tabla se exige admin.
func (s *Server) gateHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tool, id, ok := toolCallName(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		want := minRoleFor(tool)
		role, actor := s.tokenRole(r)
		if !roleAtLeast(role, want) {
			writeRPCError(w, id, "herramienta "+tool+" requiere rol "+want)
			return
		}
		if want != store.RoleViewer && actor != "" {
			if db, err := store.Open(logx.DBPath(s.cfgPath)); err == nil {
				_ = db.AddAudit(actor, "mcp "+tool, "", clientIP(r))
				db.Close()
			}
		}
		next.ServeHTTP(w, r)
	})
}
