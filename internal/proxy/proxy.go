// Package proxy: handler único Host -> ReverseProxy.
package proxy

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/netutil"
	"rkt-proxy/internal/ratelimit"
	"rkt-proxy/internal/waf"
)

// NewHandler devuelve handler único: match por Host -> ReverseProxy,
// con rate-limit por IP delante si la config lo activa.
// Lee la config viva por request (hot-reload de rutas).
func NewHandler(live *config.Live, log *slog.Logger) http.Handler {
	transport := &graceTransport{
		live: live,
		log:  log,
		base: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          200,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	// Un ReverseProxy por target vivo: se reutiliza entre requests en vez
	// de asignar uno nuevo en cada llamada.
	var proxies sync.Map // target.String() -> *httputil.ReverseProxy
	proxyFor := func(target *url.URL) *httputil.ReverseProxy {
		key := target.String()
		if v, ok := proxies.Load(key); ok {
			return v.(*httputil.ReverseProxy)
		}
		p := &httputil.ReverseProxy{
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
		actual, _ := proxies.LoadOrStore(key, p)
		return actual.(*httputil.ReverseProxy)
	}

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := live.Get()
		m := cfg.MatchRoute(r.Host, localPortOf(r), r.URL.Path)
		if m == nil || m.Target == nil {
			log.Warn("sin ruta", "host", r.Host, "remote", r.RemoteAddr, "path", r.URL.Path)
			http.Error(w, "dominio no configurado", http.StatusNotFound)
			return
		}
		target := m.Target
		if m.Strip != "" {
			// Path-based: la app ve la ruta sin el prefijo de montaje.
			rest := strings.TrimPrefix(r.URL.Path, m.Strip)
			if rest == "" || rest[0] != '/' {
				rest = "/" + rest
			}
			r.URL.Path = rest
			r.URL.RawPath = ""
		}
		if !guardApp(cfg, target, w, r, log) {
			return
		}
		start := time.Now()
		proxyFor(target).ServeHTTP(w, r)
		log.Info("proxy",
			"host", r.Host,
			"method", r.Method,
			"path", r.URL.Path,
			"target", target.Host,
			"req", r.Header.Get("X-Request-Id"),
			"dur", time.Since(start).String(),
		)
	})

	inner := http.Handler(backend)
	if rl := live.Get().RateLimit; rl.Enabled {
		// waf fuera: lo denegado no gasta tokens ni toca backend.
		inner = ratelimit.New(rl.RPS, rl.Burst, rl.Allow...).Middleware(inner, log)
	}
	inner = Compress(inner)
	inner = maxBody(live, waf.Middleware(live, inner, log), log)
	inner = recovery(log, inner)
	return RequestID(EdgeHeaders(inner))
}

// localPortOf puerto del listener que recibió el request (estrategia por
// puerto). Prefiere LocalAddr (lado servidor); sin él (handler directo en
// tests) usa el puerto declarado en Host; sin ninguno, 0 = principal.
func localPortOf(r *http.Request) int {
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr != nil {
		if _, p, err := net.SplitHostPort(addr.String()); err == nil {
			if n, err := strconv.Atoi(p); err == nil {
				return n
			}
		}
	}
	if _, p, err := net.SplitHostPort(r.Host); err == nil {
		if n, cerr := strconv.Atoi(p); cerr == nil {
			return n
		}
	}
	return 0
}

// guardApp aplica protección declarada por app: allowlist IP, mantenimiento
// y basic auth. No toca apps sin estas opciones ni rutas sin app.
func guardApp(cfg *config.Config, target *url.URL, w http.ResponseWriter, r *http.Request, log *slog.Logger) bool {
	app := cfg.AppForTarget(target.String())
	if app == nil {
		return true
	}
	ip := netutil.ClientIP(r)
	if len(app.AllowIPs) > 0 && !ipAllowed(ip, app.AllowIPs) {
		log.Warn("app ip denegada", "app", app.Name, "ip", ip)
		http.Error(w, "acceso denegado", http.StatusForbidden)
		return false
	}
	if app.Maintenance {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("<!doctype html><title>Mantenimiento</title><h1>En mantenimiento</h1>"))
		return false
	}
	if app.BasicAuth != "" {
		user, hash, _ := strings.Cut(app.BasicAuth, ":")
		u, p, ok := r.BasicAuth()
		if !ok || u != user || bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="rkt"`)
			http.Error(w, "autenticación requerida", http.StatusUnauthorized)
			return false
		}
	}
	return true
}

func ipAllowed(ip string, cidrs []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, raw := range cidrs {
		if _, n, err := net.ParseCIDR(raw); err == nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}

// graceTransport reintenta solo errores de dial hasta restart_grace. Antes
// cada request hacía un dial TCP extra para comprobar el backend.
type graceTransport struct {
	base http.RoundTripper
	live *config.Live
	log  *slog.Logger
}

func (t *graceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	grace := t.live.Get().Grace()
	deadline := time.Now().Add(grace)
	for {
		resp, err := t.base.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		if req.Context().Err() != nil || time.Now().After(deadline) || !isDialErr(err) {
			return nil, err
		}
		if req.Body != nil && req.Body != http.NoBody {
			if req.GetBody == nil {
				return nil, err
			}
			body, gerr := req.GetBody()
			if gerr != nil {
				return nil, err
			}
			req.Body = body
		}
		t.log.Info("backend aún no, reintento", "host", req.URL.Host, "err", err)
		time.Sleep(250 * time.Millisecond)
	}
}

func isDialErr(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

// recovery convierte un panic en 500 sin tumbar el servicio.
// http.ErrAbortHandler es el sentinel de net/http cuando el cliente corta
// la conexión a mitad de respuesta (p. ej. un video): no es un fallo del handler,
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
