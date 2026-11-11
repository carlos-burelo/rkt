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
	"sort"
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

// Ban un bloqueo temporal de IP para el panel.
type Ban struct {
	IP    string `json:"ip"`
	Hits  int    `json:"hits"`
	Until string `json:"until"`
}

const maxBlocks = 50

// Política de auto-ban: N denegaciones en la ventana dispara un bloqueo
// de banTTL. En memoria y efímero (un reinicio lo limpia): es fricción
// contra escaneos, no una lista negra permanente.
const (
	banThreshold = 20
	banWindow    = 5 * time.Minute
	banTTL       = 10 * time.Minute
	maxBans      = 4096
)

type banState struct {
	hits     []time.Time // denegaciones dentro de la ventana
	until    time.Time   // baneado hasta (cero = no baneado)
	lastSeen time.Time
}

var (
	ringMu sync.Mutex
	ring   []Block
	banMu  sync.Mutex
	bans   = map[string]*banState{}
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

// bannedSince informa si la IP está baneada en now.
func bannedSince(ip string, now time.Time) bool {
	banMu.Lock()
	defer banMu.Unlock()
	st, ok := bans[ip]
	if !ok || st.until.IsZero() {
		return false
	}
	if now.After(st.until) {
		delete(bans, ip)
		return false
	}
	return true
}

// Banned baneos activos (nuevos primero por vencimiento). Solo lectura.
// De paso purga registros cuya ventana ya expiró.
func Banned() []Ban {
	now := time.Now()
	banMu.Lock()
	defer banMu.Unlock()
	out := []Ban{}
	for ip, st := range bans {
		if st.until.IsZero() {
			if now.Sub(st.lastSeen) > banWindow {
				delete(bans, ip)
			}
			continue
		}
		if now.After(st.until) {
			delete(bans, ip)
			continue
		}
		out = append(out, Ban{IP: ip, Hits: len(st.hits), Until: st.until.UTC().Format(time.RFC3339)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Until > out[j].Until })
	return out
}

// Unban levanta el bloqueo de una IP. False si no estaba baneada.
func Unban(ip string) bool {
	banMu.Lock()
	defer banMu.Unlock()
	if _, ok := bans[ip]; !ok {
		return false
	}
	delete(bans, ip)
	return true
}

// hit registra una denegación y banea si alcanza el umbral en la ventana.
// Saturado (tope de IPs) suelta el golpe sin banear: fail-open, la
// disponibilidad manda sobre el bloqueo.
func hit(ip string, now time.Time) {
	banMu.Lock()
	defer banMu.Unlock()
	st, ok := bans[ip]
	if !ok {
		if len(bans) >= maxBans {
			purgeExpiredLocked(now)
			if len(bans) >= maxBans {
				return
			}
		}
		st = &banState{}
		bans[ip] = st
	}
	st.lastSeen = now
	if !st.until.IsZero() && now.Before(st.until) {
		return // ya baneado: el TTL es fijo, no se extiende
	}
	kept := st.hits[:0]
	for _, h := range st.hits {
		if now.Sub(h) < banWindow {
			kept = append(kept, h)
		}
	}
	st.hits = append(kept, now)
	if len(st.hits) >= banThreshold {
		st.until = now.Add(banTTL)
	}
}

// purgeExpiredLocked saca baneos vencidos y registros cuya ventana ya no
// cuenta. Debe llamarse con banMu tomado.
func purgeExpiredLocked(now time.Time) {
	for ip, st := range bans {
		if st.until.IsZero() {
			if now.Sub(st.lastSeen) > banWindow {
				delete(bans, ip)
			}
			continue
		}
		if now.After(st.until) {
			delete(bans, ip)
		}
	}
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

// Middleware niega baneados, verbos, paths y cabeceras de la lista viva;
// resto pasa. El ban se chequea primero: un baneado no gasta ni matching.
func Middleware(live *config.Live, next http.Handler, log *slog.Logger) http.Handler {
	clientIP := func(r *http.Request) string {
		if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			return h
		}
		return r.RemoteAddr
	}
	deny := func(w http.ResponseWriter, r *http.Request, rule string) {
		ip := clientIP(r)
		record(ip, r.Host, r.Method, r.URL.Path, rule)
		hit(ip, time.Now())
		log.Warn("waf deny", "ip", ip, "host", r.Host, "method", r.Method, "path", r.URL.Path, "rule", rule)
		http.Error(w, "prohibido", http.StatusForbidden)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bannedSince(clientIP(r), time.Now()) {
			log.Warn("waf ban", "ip", clientIP(r), "host", r.Host, "path", r.URL.Path)
			http.Error(w, "prohibido", http.StatusForbidden)
			return
		}
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
