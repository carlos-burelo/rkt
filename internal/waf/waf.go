// Package waf: bloquea paths de escaneo (/.env, /php-cgi*...)
// con 403 antes de gastar rate-limit o backend. Patrones glob
// (path.Match) sobre el path, leídos de la config viva.
package waf

import (
	"log/slog"
	"net/http"
	"path"

	"rkt-proxy/internal/config"
)

// Middleware niega paths de la lista viva; resto pasa.
func Middleware(live *config.Live, next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range live.Get().DenyPaths {
			if ok, _ := path.Match(p, r.URL.Path); ok {
				log.Warn("waf deny", "ip", r.RemoteAddr, "host", r.Host, "path", r.URL.Path)
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
