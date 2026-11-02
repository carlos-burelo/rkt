// Command rkt-proxy: dominios/subdominios -> puertos locales,
// TLS Let's Encrypt auto, servicio Windows.
//
// Uso: rkt-proxy [-config path] [install|uninstall|start|stop|run|version|logs|restart|deploy|rollback|deployments|github|apps|routes|env|settings|config|status]
package main

import (
	"flag"

	"rkt-proxy/internal/app"
)

func main() {
	configPath := flag.String("config", "config.yaml", "ruta config YAML")
	flag.Parse()
	app.Run(*configPath)
}
