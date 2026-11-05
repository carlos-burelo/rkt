// Mutaciones canónicas de flota sobre una copia de la config.
// API, CLI y MCP describen el cambio aquí; ninguno reimplementa la lógica.
package cfgfile

import (
	"fmt"
	"strings"

	"rkt-proxy/internal/config"
)

// DeployPatch campos editables del bloque deploy (token_env no se expone).
type DeployPatch struct {
	Repo  string   `json:"repo"`
	Ref   string   `json:"ref"`
	Build []string `json:"build"`
	Auto  bool     `json:"auto"`
}

// AppPatch cambio parcial de una app. nil = no tocar el campo.
type AppPatch struct {
	Command      *string      `json:"command"`
	Args         *[]string    `json:"args"`
	Dir          *string      `json:"dir"`
	EnvFile      *string      `json:"env_file"`
	Static       *string      `json:"static"`
	RestartDelay *string      `json:"restart_delay"`
	MinUptime    *string      `json:"min_uptime"`
	MemoryMB     *uint64      `json:"memory_mb"`
	Port         *int         `json:"port"`
	Domains      *[]string    `json:"domains"`
	Disabled     *bool        `json:"disabled"`
	HealthPath   *string      `json:"health_path"`
	Deploy       *DeployPatch `json:"deploy"`
}

// UpdateApp aplica un patch, valida y retargeta rutas si cambia puerto o
// dominio. No persiste: el caller decide el camino de escritura.
func UpdateApp(cp *config.Config, name string, p AppPatch) error {
	work := cp.Clone()
	app := FindApp(work, name)
	if app == nil {
		return fmt.Errorf("app %q inexistente", name)
	}
	if p.Command != nil {
		app.Command = *p.Command
	}
	if p.Args != nil {
		app.Args = *p.Args
	}
	if p.Dir != nil {
		app.Dir = *p.Dir
	}
	if p.EnvFile != nil {
		app.EnvFile = *p.EnvFile
	}
	if p.Static != nil {
		app.Static = *p.Static
	}
	if p.RestartDelay != nil {
		app.RestartDelay = *p.RestartDelay
	}
	if p.MinUptime != nil {
		app.MinUptime = *p.MinUptime
	}
	if p.MemoryMB != nil {
		app.MemoryMB = *p.MemoryMB
	}
	if p.Disabled != nil {
		app.Disabled = *p.Disabled
	}
	if p.HealthPath != nil {
		app.HealthPath = *p.HealthPath
	}
	if p.Deploy != nil {
		app.Deploy = &config.Deploy{
			Repo: p.Deploy.Repo, Ref: p.Deploy.Ref,
			Build: p.Deploy.Build, Auto: p.Deploy.Auto,
		}
	}
	if p.Port != nil {
		if err := SetAppPort(work, name, *p.Port); err != nil {
			return err
		}
	}
	if p.Domains != nil {
		if err := SetAppDomains(work, name, *p.Domains); err != nil {
			return err
		}
	}
	if err := app.ValidateApp(0); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	// Atómico: solo se publica si todo validó.
	cp.Apps, cp.Routes = work.Apps, work.Routes
	return nil
}

// RemoveApp quita una app y, salvo keepRoutes, las rutas que apuntan a su
// puerto. Devuelve cuántas rutas se retiraron.
func RemoveApp(cp *config.Config, name string, keepRoutes bool) (int, error) {
	app := FindApp(cp, name)
	if app == nil {
		return 0, fmt.Errorf("app %q inexistente", name)
	}
	target := appTarget(app)
	apps := make([]config.App, 0, len(cp.Apps))
	for _, a := range cp.Apps {
		if a.Name != name {
			apps = append(apps, a)
		}
	}
	cp.Apps = apps
	if keepRoutes || target == "" {
		return 0, nil
	}
	routes := make([]config.Route, 0, len(cp.Routes))
	dropped := 0
	for _, r := range cp.Routes {
		if r.Target == target {
			dropped++
			continue
		}
		routes = append(routes, r)
	}
	cp.Routes = routes
	return dropped, nil
}

// AddRoute agrega hosts→target y valida el conjunto (duplicados incluidos).
func AddRoute(cp *config.Config, hosts []string, target string) error {
	if len(hosts) == 0 || strings.TrimSpace(target) == "" {
		return fmt.Errorf("hosts y target requeridos")
	}
	routes := append(append([]config.Route(nil), cp.Routes...), config.Route{Hosts: hosts, Target: target})
	if err := config.ValidateRoutes(routes); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	cp.Routes = routes
	return nil
}

// RemoveRouteHost quita un host (case-insensitive) y descarta rutas vacías.
// false = el host no estaba.
func RemoveRouteHost(cp *config.Config, host string) (bool, error) {
	removed := false
	routes := make([]config.Route, 0, len(cp.Routes))
	for _, r := range cp.Routes {
		hosts := make([]string, 0, len(r.Hosts))
		for _, h := range r.Hosts {
			if strings.EqualFold(h, host) {
				removed = true
				continue
			}
			hosts = append(hosts, h)
		}
		if len(hosts) > 0 {
			r.Hosts = hosts
			routes = append(routes, r)
		}
	}
	if !removed {
		return false, nil
	}
	cp.Routes = routes
	return true, nil
}

// SetRouteTarget apunta todas las rutas que contienen host al target nuevo.
// false = el host no estaba.
func SetRouteTarget(cp *config.Config, host, target string) (bool, error) {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(target) == "" {
		return false, fmt.Errorf("host y target requeridos")
	}
	work := cp.Clone()
	found := false
	for i := range work.Routes {
		for _, h := range work.Routes[i].Hosts {
			if strings.EqualFold(h, host) {
				work.Routes[i].Target = target
				found = true
			}
		}
	}
	if !found {
		return false, nil
	}
	if err := config.ValidateRoutes(work.Routes); err != nil {
		return false, fmt.Errorf("inválido, no guardado: %w", err)
	}
	cp.Routes = work.Routes
	return true, nil
}

// UnsetEnv borra una variable de la app y valida (no persiste).
func UnsetEnv(cp *config.Config, name, key string) error {
	work := cp.Clone()
	app := FindApp(work, name)
	if app == nil {
		return fmt.Errorf("app %q inexistente", name)
	}
	if key == "" {
		return fmt.Errorf("clave vacía")
	}
	delete(app.Env, key)
	if err := app.ValidateApp(0); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	cp.Apps = work.Apps
	return nil
}
