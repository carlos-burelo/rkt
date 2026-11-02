// Command rkt-proxy: dominios/subdominios -> puertos locales,
// TLS Let's Encrypt auto, servicio Windows.
//
// Uso: rkt-proxy [-config path] [install|uninstall|start|stop|run]
package main

import (
	"flag"
	"log/slog"
	"os"

	"rkt-proxy/internal/app"
)

func main() {
	configPath := flag.String("config", "config.yaml", "ruta config YAML")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	app.Run(*configPath, logger)
}
