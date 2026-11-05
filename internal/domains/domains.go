// Package domains decide el dominio por defecto de una app según la
// instancia sea local (sin IP pública) o pública. No toca HTTP ni SQLite.
package domains

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	KindLocal = "local"
	KindBase  = "base"
)

// ErrDomainRequired se devuelve en un servidor público sin dominio utilizable.
var ErrDomainRequired = errors.New(
	"servidor con IP pública: define el dominio de la app o configura base_domain",
)

// Public true si public_ip es una IP global (no loopback, privada ni link-local).
func Public(publicIP string) bool {
	ip := net.ParseIP(strings.TrimSpace(publicIP))
	if ip == nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
}

// CleanBase normaliza el dominio base (minúsculas, sin puntos extremos).
func CleanBase(base string) string {
	return strings.Trim(strings.TrimSpace(strings.ToLower(base)), ".")
}

// IsLocal true para nombres reservados de loopback o pruebas.
func IsLocal(host string) bool {
	h := CleanBase(host)
	return h == "localhost" || strings.HasSuffix(h, ".localhost") ||
		h == "test" || strings.HasSuffix(h, ".test") ||
		h == "local" || strings.HasSuffix(h, ".local")
}

// Default dominio canónico de una app nueva.
// Local: <name>.localhost. Público: <name>.<base_domain>; sin base, error.
func Default(name, base, publicIP string) (string, error) {
	name = strings.Trim(strings.TrimSpace(strings.ToLower(name)), "-")
	if name == "" {
		return "", fmt.Errorf("dominio por defecto: nombre vacío")
	}
	if !Public(publicIP) {
		return name + ".localhost", nil
	}
	base = CleanBase(base)
	if base == "" || IsLocal(base) {
		return "", ErrDomainRequired
	}
	return name + "." + base, nil
}

// RequiresDomain true si la instancia pública no puede inferir un dominio.
func RequiresDomain(publicIP, base string) bool {
	return Public(publicIP) && (CleanBase(base) == "" || IsLocal(base))
}

// Suggestion chip de dominio ofrecido en alta y ajustes.
type Suggestion struct {
	Host string `json:"host"`
	Kind string `json:"kind"`
	Note string `json:"note"`
}

// Suggestions ordena lo que funciona según el modo de red. En local prioriza
// *.localhost; en público solo el dominio base (o nada si falta).
func Suggestions(name, base, publicIP string, used map[string]bool) []Suggestion {
	name = strings.Trim(strings.TrimSpace(strings.ToLower(name)), "-")
	if name == "" {
		return nil
	}
	base = CleanBase(base)
	out := []Suggestion{}
	add := func(host, kind, note string) {
		if host == "" || used[strings.ToLower(host)] {
			return
		}
		out = append(out, Suggestion{Host: host, Kind: kind, Note: note})
	}
	if !Public(publicIP) {
		add(name+".localhost", KindLocal, "funciona sin editar hosts")
		add(name+".test", KindLocal, "requiere hosts → 127.0.0.1")
		if base != "" {
			add(name+"."+base, KindBase, "tu dominio base")
		}
		return out
	}
	if base != "" && !IsLocal(base) {
		add(name+"."+base, KindBase, "tu dominio base")
	}
	return out
}
