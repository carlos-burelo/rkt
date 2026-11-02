// Command rkt-proxy: dominios/subdominios -> puertos locales,
// TLS Let's Encrypt auto, servicio Windows.
//
// Uso: rkt-proxy [-config path] [install|uninstall|start|stop|run|version|logs]
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
