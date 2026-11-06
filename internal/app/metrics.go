package app

import (
	"fmt"
	"sort"
	"strings"

	"rkt-proxy/internal/build"
	"rkt-proxy/internal/health"
)

// metricsText expone métricas Prometheus mínimas con datos reales:
// flota, rutas y recursos del host. Solo loopback (/metrics).
func (p *Program) metricsText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP rkt_build_info Versión del binario.\n")
	fmt.Fprintf(&b, "# TYPE rkt_build_info gauge\n")
	fmt.Fprintf(&b, "rkt_build_info{version=%q} 1\n", buildVersion())

	live := p.live.Get()
	if p.sup != nil {
		b.WriteString("# HELP rkt_app_up App corriendo (1) o parada (0).\n# TYPE rkt_app_up gauge\n")
		for _, s := range p.sup.Snapshot() {
			up := 0
			if s.Running && !s.Failed {
				up = 1
			}
			fmt.Fprintf(&b, "rkt_app_up{app=%q} %d\n", s.Name, up)
			fmt.Fprintf(&b, "rkt_app_restarts_total{app=%q} %d\n", s.Name, s.Restarts)
			failed := 0
			if s.Failed {
				failed = 1
			}
			fmt.Fprintf(&b, "rkt_app_failed{app=%q} %d\n", s.Name, failed)
		}
	}
	b.WriteString("# HELP rkt_route_backend_up Backend TCP alcanzable.\n# TYPE rkt_route_backend_up gauge\n")
	seen := map[string]bool{}
	targets := []string{}
	for i := range live.Routes {
		t := live.Routes[i].TargetURL()
		if t == nil || seen[t.Host] {
			continue
		}
		seen[t.Host] = true
		targets = append(targets, t.Host)
	}
	sort.Strings(targets)
	for _, host := range targets {
		up := 0
		if health.BackendOK(host) {
			up = 1
		}
		fmt.Fprintf(&b, "rkt_route_backend_up{target=%q} %d\n", host, up)
	}
	if p.srvMetrics != nil {
		if s, err := p.srvMetrics.Snapshot(); err == nil {
			b.WriteString("# HELP rkt_host_cpu_percent CPU del host.\n# TYPE rkt_host_cpu_percent gauge\n")
			fmt.Fprintf(&b, "rkt_host_cpu_percent %g\n", s.CPU.Percent)
			b.WriteString("# HELP rkt_host_memory_percent RAM usada.\n# TYPE rkt_host_memory_percent gauge\n")
			fmt.Fprintf(&b, "rkt_host_memory_percent %g\n", s.Memory.Percent)
			for _, d := range s.Disks {
				fmt.Fprintf(&b, "rkt_host_disk_percent{mount=%q} %g\n", d.Mount, d.Percent)
			}
			for _, t := range s.Temps {
				fmt.Fprintf(&b, "rkt_host_temp_celsius{sensor=%q} %g\n", t.Zone, t.Celsius)
			}
		}
	}
	return b.String()
}

func buildVersion() string {
	return build.Version
}
