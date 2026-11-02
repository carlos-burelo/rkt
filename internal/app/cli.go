package app

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kardianos/service"

	"rkt-proxy/internal/logx"
)

// Run cablea logger (consola+archivo), servicio Windows y despacha:
// install | uninstall | start | stop | run | version | logs [-n N] [-f].
// Sin args detecta servicio vs interactivo.
func Run(cfgPath string) {
	logger, closeLog, err := logx.Setup(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	defer closeLog()

	svcConfig := &service.Config{
		Name:        "RktProxy",
		DisplayName: "RKT Reverse Proxy",
		Description: "Reverse proxy Go: dominios -> puertos locales, TLS Let's Encrypt auto.",
		Arguments:   []string{"-config", logx.Abs(cfgPath)},
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
		case "logs":
			cmdLogs(flag.Args()[1:], cfgPath)
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

// cmdLogs imprime últimas N líneas del log y opcionalmente sigue (-f).
func cmdLogs(args []string, cfgPath string) {
	n := 50
	follow := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--follow":
			follow = true
		case "-n", "--lines":
			i++
			if i >= len(args) {
				log.Fatal("logs: -n requiere número")
			}
			if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil || n < 1 {
				log.Fatalf("logs: -n inválido %q", args[i])
			}
		default:
			log.Fatalf("logs: flag desconocido %q (uso: logs [-n 50] [-f])", args[i])
		}
	}
	path := logx.Path(cfgPath)
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("logs: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	if !follow {
		return
	}
	off := fileSize(path)
	for {
		time.Sleep(500 * time.Millisecond)
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}
		if st.Size() < off {
			off = 0 // rotó
		}
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			f.Close()
			continue
		}
		buf, _ := io.ReadAll(f)
		off += int64(len(buf))
		f.Close()
		if len(buf) > 0 {
			fmt.Print(string(buf))
		}
	}
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}
