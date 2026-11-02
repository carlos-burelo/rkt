// Package proxy: handler único Host -> ReverseProxy.
package proxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"rkt-proxy/internal/config"
)

// NewHandler devuelve handler único: match por Host -> ReverseProxy.
func NewHandler(cfg *config.Config, log *slog.Logger) http.Handler {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := cfg.Match(r.Host)
		if target == nil {
			log.Warn("sin ruta", "host", r.Host, "remote", r.RemoteAddr, "path", r.URL.Path)
			http.Error(w, "dominio no configurado", http.StatusNotFound)
			return
		}
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				// Nota: ReverseProxy ya inyecta X-Forwarded-For solo.
				// Aquí solo Proto + Host + Real-IP.
				proto := "http"
				if req.TLS != nil {
					proto = "https"
				}
				if prior := req.Header.Get("X-Forwarded-Proto"); prior != "" {
					req.Header.Set("X-Forwarded-Proto", prior+", "+proto)
				} else {
					req.Header.Set("X-Forwarded-Proto", proto)
				}
				req.Header.Set("X-Forwarded-Host", req.Host)
				req.Host = target.Host
				if ip, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
					req.Header.Set("X-Real-IP", ip)
				}
			},
			Transport: transport,
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
				log.Error("backend caído", "host", req.Host, "target", target.String(), "err", err)
				http.Error(rw, "backend no disponible", http.StatusBadGateway)
			},
		}
		start := time.Now()
		proxy.ServeHTTP(w, r)
		log.Info("proxy",
			"host", r.Host,
			"method", r.Method,
			"path", r.URL.Path,
			"target", target.Host,
			"dur", time.Since(start).String(),
		)
	})
}
