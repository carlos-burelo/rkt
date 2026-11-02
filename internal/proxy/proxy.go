// Package proxy: handler único Host -> ReverseProxy.
package proxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/ratelimit"
	"rkt-proxy/internal/waf"
)

// NewHandler devuelve handler único: match por Host -> ReverseProxy,
// con rate-limit por IP delante si la config lo activa.
// Lee la config viva por request (hot-reload de rutas).
func NewHandler(live *config.Live, log *slog.Logger) http.Handler {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
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
		// Hueco de restart: si el backend no acepta TCP, espera hasta
		// restart_grace en vez de 502 inmediato (cero fallos en deploys).
		waited := false
		deadline := start.Add(cfg.Grace())
		for !dialOpen(target.Host) {
			if time.Now().After(deadline) {
				break
			}
			if !waited {
				log.Info("backend aún no, esperando hueco", "host", r.Host, "target", target.Host)
				waited = true
			}
			time.Sleep(250 * time.Millisecond)
		}
		proxy.ServeHTTP(w, r)
		log.Info("proxy",
			"host", r.Host,
			"method", r.Method,
			"path", r.URL.Path,
			"target", target.Host,
			"dur", time.Since(start).String(),
		)
	})

	if !live.Get().RateLimit.Enabled {
		return waf.Middleware(live, backend, log)
	}
	rl := live.Get().RateLimit
	// waf fuera: lo denegado no gasta tokens ni toca backend.
	return waf.Middleware(live, ratelimit.New(rl.RPS, rl.Burst).Middleware(backend, log), log)
}

// dialOpen true si el backend acepta TCP (sondeo barato loopback).
func dialOpen(hostport string) bool {
	c, err := net.DialTimeout("tcp", hostport, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
