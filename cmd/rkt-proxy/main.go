// Command rkt-proxy: dominios/subdominios -> puertos locales,
// TLS Let's Encrypt auto, servicio Windows.
//
// Uso: rkt [-config path] [--json] [help|install|uninstall|start|stop|run|dev|version|logs|restart|deploy|rollback|deployments|github|apps|routes|env|settings|config|status]
// Códigos: 0 ok, 1 error runtime, 2 mal uso. `help` lista todo.
package main

import (
	"flag"

	"rkt-proxy/internal/cli"
)

func main() {
	configPath := flag.String("config", "rkt.db", "ancla del home: todo cuelga de <dir> (data, secrets, logs, apps, certs)")
	flag.Parse()
	cli.Run(*configPath)
}
