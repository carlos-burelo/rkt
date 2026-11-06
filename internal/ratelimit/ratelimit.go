// Package ratelimit: freno por IP con token bucket (golpe de bots
// no llega al backend). Seguro para uso concurrente.
package ratelimit

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"rkt-proxy/internal/netutil"
)

// idleTTL echa buckets sin uso; sweep cada minuto (var para tests).
const idleTTL = 3 * time.Minute

var sweepEvery = time.Minute

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// Limiter un bucket por IP.
type Limiter struct {
	mu      sync.Mutex
	clients map[string]*client
	rps     rate.Limit
	burst   int
	allow   []*net.IPNet
	done    chan struct{}
}

// New arranca limpieza de fondo. Stop la detiene (tests/apagado).
// allow lista CIDR exentos (opcional).
func New(rps float64, burst int, allow ...string) *Limiter {
	l := &Limiter{
		clients: map[string]*client{},
		rps:     rate.Limit(rps),
		burst:   burst,
		done:    make(chan struct{}),
	}
	for _, raw := range allow {
		if _, n, err := net.ParseCIDR(strings.TrimSpace(raw)); err == nil {
			l.allow = append(l.allow, n)
		}
	}
	go func() {
		t := time.NewTicker(sweepEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.sweep(time.Now())
			case <-l.done:
				return
			}
		}
	}()
	return l
}

// sweep echa buckets sin uso (una pasada; testeable sin esperar 1 minuto).
func (l *Limiter) sweep(now time.Time) {
	l.mu.Lock()
	for ip, c := range l.clients {
		if now.Sub(c.lastSeen) > idleTTL {
			delete(l.clients, ip)
		}
	}
	l.mu.Unlock()
}

// Stop detiene la limpieza.
func (l *Limiter) Stop() {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
}

// Allow true si la IP tiene token.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, ok := l.clients[ip]
	if !ok {
		c = &client{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = time.Now()
	return c.limiter.Allow()
}

func (l *Limiter) ipAllowed(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range l.allow {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// Middleware responde 429 sin tocar el backend. Bucket por (IP, host):
// una app pesada no consume el cupo de las demás para esa IP.
func (l *Limiter) Middleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := netutil.ClientIP(r)
		if l.ipAllowed(ip) {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !l.Allow(ip + "|" + strings.ToLower(host)) {
			log.Warn("rate limit", "ip", ip, "host", host, "path", r.URL.Path)
			w.Header().Set("Retry-After", "1")
			http.Error(w, "demasiadas peticiones", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
