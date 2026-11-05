// Mutaciones de una app sobre la flota copiada (routes incluidas).
package cfgfile

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"rkt-proxy/internal/config"
)

// appTarget devuelve el target del proxy para el puerto de la app.
func appTarget(app *config.App) string {
	port := app.Env["PORT"]
	if port == "" {
		return ""
	}
	return "http://127.0.0.1:" + port
}

// SetAppDomains reemplaza el conjunto de hosts que apuntan al puerto de la
// app. Una app puede tener varios hosts; viven en una sola ruta. Lista vacía
// retira todas las rutas del target. El primer host es el principal visual.
func SetAppDomains(cp *config.Config, appName string, hosts []string) error {
	app := FindApp(cp, appName)
	if app == nil {
		return fmt.Errorf("app %q inexistente", appName)
	}
	target := appTarget(app)
	if target == "" {
		return fmt.Errorf("app %q sin puerto (define port primero)", appName)
	}
	normalized := make([]string, 0, len(hosts))
	seen := map[string]bool{}
	for _, raw := range hosts {
		host := normalizeHost(raw)
		if host == "" {
			continue
		}
		if strings.ContainsAny(host, " /\\") {
			return fmt.Errorf("dominio inválido: %q", raw)
		}
		if u, err := url.Parse("http://" + host); err != nil || u.Host != host {
			return fmt.Errorf("dominio inválido: %q", raw)
		}
		if seen[host] {
			continue
		}
		seen[host] = true
		normalized = append(normalized, host)
	}
	routes := dropTarget(cp.Routes, target)
	if len(normalized) > 0 {
		routes = append(routes, config.Route{Hosts: normalized, Target: target})
	}
	if err := config.ValidateRoutes(routes); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	cp.Routes = routes
	return nil
}

// SetAppPort cambia el puerto de la app y retargeta sus rutas.
func SetAppPort(cp *config.Config, appName string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("puerto fuera de rango: %d", port)
	}
	app := FindApp(cp, appName)
	if app == nil {
		return fmt.Errorf("app %q inexistente", appName)
	}
	old := appTarget(app)
	if app.Env == nil {
		app.Env = map[string]string{}
	}
	app.Env["PORT"] = strconv.Itoa(port)
	next := appTarget(app)
	if old != "" && old != next {
		for i := range cp.Routes {
			if cp.Routes[i].Target == old {
				cp.Routes[i].Target = next
			}
		}
	}
	return nil
}

// dropTarget quita hosts del target. Si una ruta mezcla hosts de varios
// targets (no debería), conserva los demás hosts en su propia ruta.
func dropTarget(routes []config.Route, target string) []config.Route {
	out := make([]config.Route, 0, len(routes))
	for _, r := range routes {
		if r.Target != target {
			out = append(out, r)
		}
	}
	return out
}

func normalizeHost(host string) string {
	h := strings.TrimSpace(strings.ToLower(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	return strings.TrimSuffix(h, ".")
}
