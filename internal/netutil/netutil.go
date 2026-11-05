// Package netutil: operaciones de red compartidas por transportes y runtime.
// Un solo parser de listen/RemoteAddr para no divergir entre API y servicio.
package netutil

import (
	"net"
	"net/http"
	"strings"
)

// Port extrae el puerto de ":80", "127.0.0.1:8080" o "[::1]:8090".
// Sin puerto parseable devuelve def.
func Port(addr, def string) string {
	if addr == "" {
		return def
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	if i := strings.LastIndex(addr, ":"); i >= 0 && i+1 < len(addr) {
		return addr[i+1:]
	}
	return def
}

// ClientIP devuelve el host de RemoteAddr, o RemoteAddr si no trae puerto.
func ClientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}
