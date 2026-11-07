// Package waf: bloquea paths de escaneo (/.env, /php-cgi*...)
// con 403 antes de gastar rate-limit o backend. Patrones glob
// (path.Match) sobre el path, leídos de la config viva.
// Además niega cabeceras internas del framework que jamás manda un
// navegador (x-middleware-subrequest: bypass de middleware,
// CVE-2025-29927); si llegan de fuera, son ataque.
package waf

import (
	"log/slog"
	"net/http"
	"path"
	"strings"

	"rkt-proxy/internal/config"
)

// blockedRequestHeaders jamás legítimas desde un cliente real.
var blockedRequestHeaders = []string{"X-Middleware-Subrequest"}

// matchPattern prueba el glob contra el path completo y, si el patrón
// no tiene "/" interior (ej "/*.php"), contra el nombre base: cubre
// cualquier profundidad sin explosión de patrones.
func matchPattern(pattern, target string) bool {
	if ok, _ := path.Match(pattern, target); ok {
		return true
	}
	if strings.HasPrefix(pattern, "/") && !strings.Contains(pattern[1:], "/") {
		if ok, _ := path.Match(pattern[1:], path.Base(target)); ok {
			return true
		}
	}
	return false
}

// Middleware niega paths y cabeceras de la lista viva; resto pasa.
func Middleware(live *config.Live, next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range live.Get().DenyPaths {
			if matchPattern(p, r.URL.Path) {
				log.Warn("waf deny", "ip", r.RemoteAddr, "host", r.Host, "path", r.URL.Path)
				http.Error(w, "prohibido", http.StatusForbidden)
				return
			}
		}
		for _, h := range blockedRequestHeaders {
			if r.Header.Get(h) != "" {
				log.Warn("waf deny header", "ip", r.RemoteAddr, "host", r.Host, "header", h, "path", r.URL.Path)
				http.Error(w, "prohibido", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ValidPattern false si el glob no compila.
func ValidPattern(p string) bool {
	_, err := path.Match(p, "")
	return err == nil
}
