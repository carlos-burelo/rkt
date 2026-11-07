package app

import (
	"net"
	"net/http"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

// mcpAuth exige un API token rkt_ de rol operator/admin. Las sesiones de
// navegador no valen: el MCP remoto es para agentes/automatización.
func mcpAuth(db *store.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "MCP requiere API key Bearer", http.StatusUnauthorized)
			return
		}
		_, role, ok := db.TokenUser(strings.TrimPrefix(h, "Bearer "))
		if !ok {
			http.Error(w, "API key inválida o expirada", http.StatusUnauthorized)
			return
		}
		// El MCP expone todas las tools (incluida gestión de usuarios):
		// solo admin para no escalar privilegios desde una key operator.
		if role != store.RoleAdmin {
			http.Error(w, "API key MCP requiere rol admin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// portOf extrae el puerto de host:puerto ("" si no hay).
func portOf(addr string) string {
	if _, p, err := net.SplitHostPort(addr); err == nil {
		return p
	}
	return ""
}

// isLoopbackRemote true si la petición viene de loopback.
func isLoopbackRemote(r *http.Request) bool {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostOnly quita el puerto de un Host header.
func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(strings.TrimSpace(h))
}

// dashboardGate sirve la consola (SPA + API) cuando el Host es el
// configurado. El resto sigue al proxy normal.
func dashboardGate(live *config.Live, apiH, dashH, mcpH, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := strings.ToLower(strings.TrimSpace(live.Get().Dashboard.Host))
		if want != "" && hostOnly(r.Host) == want {
			if r.URL.Path == "/mcp" {
				mcpH.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				apiH.ServeHTTP(w, r)
				return
			}
			dashH.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// consoleOnly sirve solo SPA + API. Nunca healthz, métricas ni acciones
// de control: ese plano se queda en loopback.
func consoleOnly(apiH, dashH, mcpH http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mcp" {
			mcpH.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiH.ServeHTTP(w, r)
			return
		}
		dashH.ServeHTTP(w, r)
	})
}

// dashboardReturnURL destino tras el setup de GitHub: público si está
// configurado, loopback en su defecto.
func dashboardReturnURL(cfg *config.Config) string {
	if h := strings.TrimSpace(cfg.Dashboard.Host); h != "" {
		scheme := "https"
		if cfg.ListenHTTPS == "" && !cfg.LocalCA {
			scheme = "http"
		}
		return scheme + "://" + h + "/settings?github=ok"
	}
	return dashboardURL(cfg.Healthz.Listen)
}
