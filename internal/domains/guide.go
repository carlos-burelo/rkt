package domains

import (
	"fmt"
	"strings"

	"rkt/internal/dnscheck"
)

// Estados de la guía de DNS.
const (
	GuideOK         = "ok"           // el A apunta a este servidor
	GuideNoRecord   = "no-record"    // el host no resuelve
	GuideWrongIP    = "wrong-ip"     // resuelve a otra IP (o proxy CDN)
	GuideCNAME      = "cname"        // CNAME hacia otro host, no A directo
	GuideIPv6Only   = "ipv6-only"    // solo AAAA
	GuideLocal      = "local"        // *.localhost / *.test / *.local
	GuideNoPublicIP = "no-public-ip" // rkt no conoce su IP pública
)

// Record registro DNS que el usuario debe crear.
type Record struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	TTL   int    `json:"ttl"`
}

// Step paso del proceso. Done lo calcula la guía contra el DNS observado.
type Step struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Done   bool   `json:"done"`
}

// Guide explicación accionable del estado DNS de un host. Misma salida
// para dashboard y MCP (agentes de soporte remoto).
type Guide struct {
	Host     string          `json:"host"`
	Status   string          `json:"status"`
	Ready    bool            `json:"ready"`
	Summary  string          `json:"summary"`
	Record   *Record         `json:"record,omitempty"`
	Wildcard *Record         `json:"wildcard,omitempty"`
	Observed dnscheck.Result `json:"observed"`
	Steps    []Step          `json:"steps"`
}

// BuildGuide arma la guía. Pura: dns viene resuelto por el llamador.
func BuildGuide(host, base, publicIP string, dns dnscheck.Result) Guide {
	h := CleanBase(host)
	base = CleanBase(base)
	g := Guide{Host: h, Observed: dns, Steps: []Step{}}

	if IsLocal(h) {
		g.Status, g.Ready = GuideLocal, true
		g.Summary = h + " es un nombre local: no necesita registro DNS público."
		g.Steps = append(g.Steps, Step{"local", "Nada que configurar",
			"*.localhost resuelve solo. Para *.test o *.local ejecuta: rkt hosts add " + h, true})
		return g
	}
	if !Public(publicIP) {
		g.Status = GuideNoPublicIP
		g.Summary = "rkt no conoce su IP pública: no puede decirte a dónde apuntar " + h + "."
		g.Steps = append(g.Steps, Step{"public-ip", "Define la IP pública del servidor",
			"Ejecuta: rkt set public_ip <IP-publica>", false})
		return g
	}

	g.Record = &Record{Type: "A", Name: recordName(h, base), Value: publicIP, TTL: 300}
	if base != "" && h != base && strings.HasSuffix(h, "."+base) {
		g.Wildcard = &Record{Type: "A", Name: "*", Value: publicIP, TTL: 300}
	}

	switch {
	case dns.Matches(publicIP):
		g.Status, g.Ready = GuideOK, true
		g.Summary = fmt.Sprintf("%s apunta a %s. El certificado TLS se emite solo en la primera visita.", h, publicIP)
	case dns.CNAME != "":
		g.Status = GuideCNAME
		g.Summary = fmt.Sprintf("%s es un CNAME hacia %s. rkt necesita un registro A directo a %s.", h, dns.CNAME, publicIP)
	case len(dns.A) == 0 && len(dns.AAAA) > 0:
		g.Status = GuideIPv6Only
		g.Summary = fmt.Sprintf("%s solo tiene AAAA (%s). Añade el A a %s o rkt no recibirá el tráfico IPv4.", h, strings.Join(dns.AAAA, ","), publicIP)
	case dns.Resolves:
		g.Status = GuideWrongIP
		g.Summary = fmt.Sprintf("%s resuelve a %s pero este servidor es %s. Si usas un proxy CDN (nube naranja) desactívalo: el webhook y ACME necesitan el A directo.",
			h, strings.Join(dns.A, ","), publicIP)
	default:
		g.Status = GuideNoRecord
		g.Summary = h + " no resuelve: falta el registro A."
	}

	ok := g.Ready
	g.Steps = []Step{
		{"create", "Crea el registro A en tu proveedor de DNS",
			fmt.Sprintf("Tipo A · nombre %s · valor %s · TTL 300 (donde compraste el dominio: Cloudflare, Namecheap, GoDaddy...).", g.Record.Name, publicIP), ok},
		{"propagate", "Espera la propagación",
			"Suele tardar 1–5 min con TTL 300 (hasta 48 h si el dominio es nuevo). rkt re-chequea cuando pulsas verificar.", ok},
		{"tls", "TLS automático",
			"Con el A correcto y los puertos 80/443 abiertos, rkt emite el certificado Let's Encrypt en la primera visita.", ok},
	}
	if g.Wildcard != nil {
		g.Steps = append(g.Steps, Step{"wildcard", "Opcional: un solo registro para todas las apps",
			fmt.Sprintf("Crea A * → %s y cualquier <app>.%s funcionará sin más pasos.", publicIP, base), false})
	}
	return g
}

// recordName nombre relativo del registro: "@" en el apex, la etiqueta
// bajo base_domain, o el prefijo asumiendo zona de dos etiquetas.
func recordName(host, base string) string {
	if host == base {
		return "@"
	}
	if base != "" && strings.HasSuffix(host, "."+base) {
		return strings.TrimSuffix(host, "."+base)
	}
	labels := strings.Split(host, ".")
	if len(labels) <= 2 {
		return "@"
	}
	return strings.Join(labels[:len(labels)-2], ".")
}
