// Package proxy: handler único Host -> ReverseProxy.
package proxy

import (
	"errors"
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
		ResponseHeaderTimeout: 30 * time.Second,
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
				// El edge no confía en cabeceras de reenvío del cliente:
				// se borran y se escriben solo las calculadas aquí.
				// ReverseProxy añade X-Forwarded-For desde RemoteAddr
				// después del Director; sin el valor previo no hay spoof.
				for _, h := range []string{
					"Forwarded", "X-Forwarded-For", "X-Forwarded-Host",
					"X-Forwarded-Proto", "X-Forwarded-Port", "X-Real-IP",
				} {
					req.Header.Del(h)
				}
				proto := "http"
				if req.TLS != nil {
					proto = "https"
				}
				req.Header.Set("X-Forwarded-Proto", proto)
				req.Header.Set("X-Forwarded-Host", req.Host)
				req.Host = target.Host
				if ip, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
					req.Header.Set("X-Real-IP", ip)
				}
			},
			Transport: transport,
			// -1 fuerza flush inmediato por escritura: SSE y streaming no
			// esperan a que termine la respuesta.
			FlushInterval: -1,
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
				// Cliente que corta (cerrar pestaña, cancelar video) no es un
				// backend caído: se registra en debug y no se responde nada.
				if req.Context().Err() != nil {
					log.Debug("petición cancelada por el cliente", "host", req.Host, "path", req.URL.Path)
					return
				}
				var tooBig *http.MaxBytesError
				if errors.As(err, &tooBig) {
					log.Warn("body por encima del tope", "host", req.Host, "path", req.URL.Path, "limit", tooBig.Limit)
					http.Error(rw, "body muy grande", http.StatusRequestEntityTooLarge)
					return
				}
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
		return secureChain(live, backend, log)
	}
	rl := live.Get().RateLimit
	// waf fuera: lo denegado no gasta tokens ni toca backend.
	return secureChain(live, ratelimit.New(rl.RPS, rl.Burst).Middleware(backend, log), log)
}

// secureChain envuelve con recovery (anti-panic) + tope de body + waf.
func secureChain(live *config.Live, next http.Handler, log *slog.Logger) http.Handler {
	return recovery(log, maxBody(live, waf.Middleware(live, next, log), log))
}

// recovery convierte un panic en 500 sin tumbar el servicio.
// http.ErrAbortHandler es el sentinel de net/http cuando el cliente corta la
// conexión a mitad de respuesta (p. ej. un video): no es un fallo del handler,
// así que no se escribe 500 ni se ensucia el log.
func recovery(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				log.Debug("petición abortada por el cliente", "host", r.Host, "path", r.URL.Path)
				return
			}
			log.Error("panic recuperado", "v", v, "host", r.Host, "path", r.URL.Path)
			http.Error(w, "error interno", http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

// maxBody rechaza bodies gigantes (Content-Length) y acota chunked.
func maxBody(live *config.Live, next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := live.Get().MaxBodyMB << 20
		if limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > limit {
			log.Warn("body gigante", "ip", r.RemoteAddr, "host", r.Host, "bytes", r.ContentLength)
			http.Error(w, "body muy grande", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
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
