// Package waf: bloquea paths de escaneo (/.env, /php-cgi*...) y verbos
// peligrosos (TRACE/TRACK/CONNECT) con 403 antes de gastar rate-limit o
// backend. Patrones glob (path.Match) sobre el path, leídos de la config
// viva. Además niega cabeceras internas del framework que jamás manda un
// navegador (x-middleware-subrequest: bypass de middleware,
// CVE-2025-29927); si llegan de fuera, son ataque.
// Guarda los últimos bloqueos en memoria para el panel (sin persistir:
// son telemetría efímera, no historial).
package waf

import (
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"rkt-proxy/internal/config"
)

// Block un bloqueo para el panel.
type Block struct {
	At     string `json:"at"`
	IP     string `json:"ip"`
	Host   string `json:"host"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Rule   string `json:"rule"`
}

const maxBlocks = 50

var (
	ringMu sync.Mutex
	ring   []Block
)

// record guarda un bloqueo (anillo: solo los últimos maxBlocks).
func record(ip, host, method, path, rule string) {
	ringMu.Lock()
	defer ringMu.Unlock()
	ring = append(ring, Block{
		At: time.Now().UTC().Format(time.RFC3339),
		IP: ip, Host: host, Method: method, Path: path, Rule: rule,
	})
	if len(ring) > maxBlocks {
		ring = append(ring[:0], ring[len(ring)-maxBlocks:]...)
	}
}

// Recent últimos bloqueos (nuevos primero). Solo lectura para el panel.
func Recent() []Block {
	ringMu.Lock()
	defer ringMu.Unlock()
	out := make([]Block, len(ring))
	for i, b := range ring {
		out[len(ring)-1-i] = b
	}
	return out
}

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

// matchRequest el path tal cual y el escapado: un glob debe cazar tanto
// /x/../.env como su forma codificada /x/%2e%2e/.env.
func matchRequest(pattern string, r *http.Request) bool {
	if matchPattern(pattern, r.URL.Path) {
		return true
	}
	if esc := r.URL.EscapedPath(); esc != r.URL.Path && matchPattern(pattern, esc) {
		return true
	}
	return false
}

// Middleware niega verbos, paths y cabeceras de la lista viva; resto pasa.
func Middleware(live *config.Live, next http.Handler, log *slog.Logger) http.Handler {
	deny := func(w http.ResponseWriter, r *http.Request, rule string) {
		ip := r.RemoteAddr
		if h, _, err := net.SplitHostPort(ip); err == nil {
			ip = h
		}
		record(ip, r.Host, r.Method, r.URL.Path, rule)
		log.Warn("waf deny", "ip", ip, "host", r.Host, "method", r.Method, "path", r.URL.Path, "rule", rule)
		http.Error(w, "prohibido", http.StatusForbidden)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, m := range live.Get().DenyMethods {
			if r.Method == m {
				deny(w, r, "método "+m)
				return
			}
		}
		for _, p := range live.Get().DenyPaths {
			if matchRequest(p, r) {
				deny(w, r, p)
				return
			}
		}
		for _, h := range blockedRequestHeaders {
			if r.Header.Get(h) != "" {
				deny(w, r, "cabecera "+h)
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
