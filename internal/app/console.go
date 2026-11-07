package app

import (
	"net"
	"net/http"
	"strings"

	"rkt-proxy/internal/config"
)

// hostOnly quita el puerto de un Host header.
func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.ToLower(host)
	}
	return strings.ToLower(strings.TrimSpace(h))
}

// dashboardGate sirve la consola (SPA + API) cuando el Host es el
// configurado. El resto sigue al proxy normal.
func dashboardGate(live *config.Live, apiH, dashH, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := strings.ToLower(strings.TrimSpace(live.Get().Dashboard.Host))
		if want != "" && hostOnly(r.Host) == want {
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
func consoleOnly(apiH, dashH http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
