package app

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/kardianos/service"
)

// Version la inyecta CI con -X en cada tag. Local: "dev".
var Version = "dev"

// Run cablea el servicio Windows y despacha subcomandos:
// install | uninstall | start | stop | run. Sin args detecta
// servicio vs interactivo.
func Run(cfgPath string, logger *slog.Logger) {
	svcConfig := &service.Config{
		Name:        "RktProxy",
		DisplayName: "RKT Reverse Proxy",
		Description: "Reverse proxy Go: dominios -> puertos locales, TLS Let's Encrypt auto.",
		Arguments:   []string{"-config", absolutePath(cfgPath)},
	}
	prg := New(cfgPath, logger)
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if flag.NArg() > 0 {
		switch flag.Arg(0) {
		case "install":
			if err := s.Install(); err != nil {
				log.Fatalf("install: %v", err)
			}
			fmt.Println("servicio instalado. Arranca con: sc start RktProxy")
			return
		case "uninstall":
			_ = s.Stop()
			if err := s.Uninstall(); err != nil {
				log.Fatalf("uninstall: %v", err)
			}
			fmt.Println("servicio desinstalado")
			return
		case "start":
			if err := s.Start(); err != nil {
				log.Fatalf("start: %v", err)
			}
			fmt.Println("servicio arrancado")
			return
		case "stop":
			if err := s.Stop(); err != nil {
				log.Fatalf("stop: %v", err)
			}
			fmt.Println("servicio detenido")
			return
		case "run":
			// foreground, sin instalar (debug / dev)
			prg.Run()
			return
		case "version":
			fmt.Println(Version)
			return
		}
	}

	if service.Interactive() {
		fmt.Println("modo interactivo: usa 'run' para foreground o install/start como servicio")
		prg.Run()
		return
	}
	if err := s.Run(); err != nil {
		logger.Error("servicio muerto", "err", err)
		os.Exit(1)
	}
}

// absolutePath resuelve la config junto al .exe: el servicio
// Windows arranca en System32 y la ruta relativa rompería.
func absolutePath(p string) string {
	if len(p) > 1 && (p[1] == ':' || p[0] == '/' || p[0] == '\\') {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return p
	}
	for i := len(exe) - 1; i >= 0; i-- {
		if exe[i] == '\\' || exe[i] == '/' {
			return exe[:i+1] + p
		}
	}
	return p
}
