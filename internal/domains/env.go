package domains

import (
	"fmt"
	"net"
)

// Tipos de entorno según IP pública, base_domain y DNS.
const (
	EnvLocal     = "local"       // sin IP pública: loopback / LAN
	EnvPublicIP  = "ip-publica"  // IP pública sin dominio utilizable
	EnvBadDomain = "dominio-mal" // dominio que no apunta a este servidor
	EnvProd      = "prod"        // dominio resuelto a este servidor
)

// Env veredicto de entorno. Problem y Fix solo vienen si Blocked().
type Env struct {
	Kind    string `json:"kind"`
	Problem string `json:"problem,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// Blocked true si crear la GitHub App (u otra URL pública) fallaría.
func (e Env) Blocked() bool { return e.Kind == EnvPublicIP || e.Kind == EnvBadDomain }

// Classify decide el entorno. baseA son los registros A resueltos de
// base_domain (el llamador resuelve; aquí no hay red). Función pura.
func Classify(base, publicIP string, baseA []string) Env {
	base = CleanBase(base)
	ip := net.ParseIP(base)
	if ip != nil {
		return Env{
			Kind:    EnvBadDomain,
			Problem: fmt.Sprintf("base_domain es una IP (%s): ACME y GitHub exigen un nombre de host", base),
			Fix:     setBaseFix(publicIP),
		}
	}
	if !Public(publicIP) {
		return Env{Kind: EnvLocal}
	}
	if base == "" || IsLocal(base) {
		return Env{
			Kind:    EnvPublicIP,
			Problem: fmt.Sprintf("servidor con IP pública %s y sin base_domain: GitHub no puede volver a un origen local", publicIP),
			Fix:     setBaseFix(publicIP),
		}
	}
	if len(baseA) == 0 {
		return Env{
			Kind:    EnvBadDomain,
			Problem: fmt.Sprintf("%s no resuelve (sin registro A)", base),
			Fix:     fmt.Sprintf("crea un registro A %s → %s y reintenta", base, publicIP),
		}
	}
	for _, a := range baseA {
		if a == publicIP {
			return Env{Kind: EnvProd}
		}
	}
	return Env{
		Kind:    EnvBadDomain,
		Problem: fmt.Sprintf("%s resuelve a %v pero este servidor es %s", base, baseA, publicIP),
		Fix:     fmt.Sprintf("apunta el registro A %s → %s (sin proxy CDN para el webhook)", base, publicIP),
	}
}

func setBaseFix(publicIP string) string {
	target := publicIP
	if target == "" {
		target = "<IP-publica>"
	}
	return fmt.Sprintf("crea un registro A <dominio> → %s y ejecuta: rkt set base_domain <dominio>", target)
}
