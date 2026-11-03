// Package appview: vistas compartidas de flota para API y MCP (una sola
// forma de describir app, rutas y zonas; nada de duplicar en cada cliente).
package appview

import (
	"net"
	"sort"
	"strings"

	"rkt-proxy/internal/buildpack"
	"rkt-proxy/internal/config"
)

// Port devuelve el puerto local declarado en Env.
func Port(a config.App) string { return a.Env["PORT"] }

// Domains hosts cuyas rutas apuntan al puerto de la app (mapeo real).
func Domains(cfg *config.Config, a config.App) []string {
	out := []string{}
	port := Port(a)
	if port == "" {
		return out
	}
	want := "http://127.0.0.1:" + port
	for _, r := range cfg.Routes {
		if r.Target == want {
			out = append(out, r.Hosts...)
		}
	}
	return out
}

// App describe una unidad para clientes (sin secretos de Env).
func App(cfg *config.Config, a config.App) map[string]any {
	static := a.Static
	if static == "" && len(a.Args) >= 2 && a.Args[0] == "__static" {
		static = a.Args[1]
	}
	var deploy any
	if a.Deploy != nil {
		deploy = map[string]any{
			"repo": a.Deploy.Repo, "ref": a.Deploy.Ref,
			"build": a.Deploy.Build, "auto": a.Deploy.Auto,
		}
	}
	domains := Domains(cfg, a)
	dnsOK := true
	for _, h := range domains {
		if !ResolvesLocal(h) {
			dnsOK = false
		}
	}
	runCommand := ""
	if a.External() {
		runCommand = buildpack.DevHint(a.Dir, Port(a))
	}
	return map[string]any{
		"name": a.Name, "runtime": a.Runtime, "external": a.External(),
		"command": a.Command, "args": a.Args,
		"dir": a.Dir, "static": static, "env_file": a.EnvFile,
		"restart_delay": a.RestartDelay, "min_uptime": a.MinUptime,
		"memory_mb": a.MemoryMB, "port": Port(a),
		"deploy": deploy, "domains": domains, "dns_ok": dnsOK,
		"run_command": runCommand,
	}
}

// Apps describe toda la flota en orden de manifiesto.
func Apps(cfg *config.Config) []map[string]any {
	out := make([]map[string]any, 0, len(cfg.Apps))
	for _, a := range cfg.Apps {
		out = append(out, App(cfg, a))
	}
	return out
}

// ResolvesLocal: el host apunta a loopback desde esta máquina.
// *.localhost lo resuelven los navegadores sin hosts; el resto se pregunta
// al resolver del sistema (si no está en hosts, no es alcanzable).
func ResolvesLocal(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ips, err := net.LookupHost(h)
	if err != nil {
		return false
	}
	return anyLoopback(ips)
}

// anyLoopback true si alguna IP es loopback (testeable sin DNS).
func anyLoopback(ips []string) bool {
	for _, ip := range ips {
		if ip == "127.0.0.1" || ip == "::1" {
			return true
		}
	}
	return false
}

// Zones agrupa rutas locales como panel DNS (apex + subdominios).

type ZoneHost struct {
	Host   string `json:"host"`
	Target string `json:"target"`
	IsBase bool   `json:"is_base"`
}

type ZoneGroup struct {
	Base   string     `json:"base"`
	Suffix string     `json:"suffix"`
	Target string     `json:"target"`
	Hosts  []ZoneHost `json:"hosts"`
}

type Zones struct {
	Suffixes []string    `json:"suffixes"`
	Groups   []ZoneGroup `json:"groups"`
}

// BuildZones puro: sufijos locales + grupos por dominio base.
func BuildZones(routes []config.Route, customBase string) Zones {
	suffixes := []string{"localhost", "test"}
	if b := strings.Trim(strings.TrimSpace(customBase), "."); b != "" && b != "localhost" && b != "test" {
		suffixes = append(suffixes, b)
	}
	target := map[string]string{}
	for _, rt := range routes {
		for _, h := range rt.Hosts {
			target[strings.ToLower(h)] = rt.Target
		}
	}
	groups := map[string]*ZoneGroup{}
	for host, tgt := range target {
		suffix, rest := splitLocal(host, suffixes)
		if suffix == "" {
			continue
		}
		base := host
		if strings.Contains(suffix, ".") {
			base = suffix
		} else if i := strings.Index(rest, "."); i >= 0 {
			base = rest[i+1:] + "." + suffix
		}
		g, ok := groups[base]
		if !ok {
			g = &ZoneGroup{Base: base, Suffix: suffix, Hosts: []ZoneHost{}}
			groups[base] = g
		}
		if host == base {
			g.Target = tgt
		}
		g.Hosts = append(g.Hosts, ZoneHost{Host: host, Target: tgt, IsBase: host == base})
	}
	out := Zones{Suffixes: suffixes, Groups: []ZoneGroup{}}
	for _, g := range groups {
		sort.Slice(g.Hosts, func(i, j int) bool { return lessHost(g.Hosts[i], g.Hosts[j]) })
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Base < out.Groups[j].Base })
	return out
}

// lessHost ordena apex primero y luego por host (testeable directo).
func lessHost(a, b ZoneHost) bool {
	if a.IsBase != b.IsBase {
		return a.IsBase
	}
	return a.Host < b.Host
}

func splitLocal(host string, suffixes []string) (suffix, rest string) {
	for _, s := range suffixes {
		if strings.HasSuffix(host, "."+s) {
			return s, strings.TrimSuffix(host, "."+s)
		}
	}
	return "", ""
}
