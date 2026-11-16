package app

import (
	"log/slog"
	"net"
	"strings"
	"time"

	"rkt/internal/cfgfile"
	"rkt/internal/config"
	"rkt/internal/discover"
	"rkt/internal/store"
)

// portBusyQuick true si algo ya escucha en 127.0.0.1:puerto. Un error de
// conexión se lee como libre: solo reubica ante ocupación probada.
func portBusyQuick(port string) bool {
	port = strings.TrimSpace(port)
	if port == "" {
		return false
	}
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// reallocateBusyPorts reubica apps detenidas cuyo PORT grabado está ocupado
// por otro proceso (TOCTOU del pool + reinicios). No toca externas (puerto
// ajeno), deshabilitadas (no arrancan) ni en ejecución (el puerto es el
// suyo). Persiste vía MutateFleet (retargeta rutas con SetAppPort).
// Devuelve la config resultante y si hubo cambios.
func reallocateBusyPorts(db *store.DB, apps []config.App, running map[string]bool, log *slog.Logger) (*config.Config, bool) {
	if db == nil {
		return nil, false
	}
	var busy []string
	oldPorts := map[string]string{}
	for i := range apps {
		a := &apps[i]
		if a.External() || a.Disabled || running[a.Name] {
			continue
		}
		if p := strings.TrimSpace(a.Env["PORT"]); p != "" && portBusyQuick(p) {
			busy = append(busy, a.Name)
			oldPorts[a.Name] = p
		}
	}
	if len(busy) == 0 {
		return nil, false
	}
	cp, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		for _, name := range busy {
			p, err := discover.FreePort(db)
			if err != nil {
				return err
			}
			if err := cfgfile.SetAppPort(cp, name, p); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Warn("no pude reubicar puertos ocupados", "apps", busy, "err", err)
		return nil, false
	}
	for _, name := range busy {
		var now string
		if a := cfgfile.FindApp(cp, name); a != nil {
			now = a.Env["PORT"]
		}
		log.Warn("puerto ocupado, app reubicada", "app", name, "antes", oldPorts[name], "ahora", now)
	}
	return cp, true
}
