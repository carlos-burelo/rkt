package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/kardianos/service"

	"rkt-proxy/internal/config"
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
			// foreground: Ctrl+C apaga servidores + apps (sin huérfanos).
			ctx, sigstop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer sigstop()
			go func() {
				<-ctx.Done()
				_ = prg.Stop(nil)
				os.Exit(0)
			}()
			prg.Run()
			return
		case "version":
			fmt.Println(Version)
			return
		case "restart":
			cmdRestart(flag.Args()[1:], cfgPath)
			return
		case "deploy":
			cmdDeploy(flag.Args()[1:], cfgPath)
			return
		case "rollback":
			cmdRollback(flag.Args()[1:], cfgPath)
			return
		case "deployments":
			cmdDeployments(flag.Args()[1:], cfgPath)
			return
		case "github":
			cmdGithub(flag.Args()[1:], cfgPath)
			return
		case "settings":
			cmdSettings(flag.Args()[1:], cfgPath)
			return
		case "config":
			cmdConfig(flag.Args()[1:], cfgPath)
			return
		case "apps":
			cmdApps(flag.Args()[1:], cfgPath)
			return
		case "routes":
			cmdRoutes(flag.Args()[1:], cfgPath)
			return
		case "env":
			cmdEnv(flag.Args()[1:], cfgPath)
			return
		case "status":
			cmdStatus(cfgPath)
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

// cmdRestart pide al servicio (vía healthz loopback) reinicio sin cortes:
// rkt-proxy restart <app>
func cmdRestart(args []string, cfgPath string) {
	if len(args) != 1 || args[0] == "" {
		log.Fatal("uso: restart <app>")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("restart: %v", err)
	}
	if !cfg.Healthz.Enabled {
		log.Fatal("restart: healthz desactivado en config")
	}
	resp, err := http.Post("http://"+cfg.Healthz.Listen+"/restart/"+args[0], "application/json", nil)
	if err != nil {
		log.Fatalf("restart: sin contacto con el servicio: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		log.Fatalf("restart: app %q inexistente", args[0])
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("restart: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	fmt.Printf("app %s reiniciando (sin cortes hasta restart_grace)\n", args[0])
}

// cmdDeploy publica una app desde git: rkt-proxy deploy <app>
// Dispara el deploy EN el servicio (vía healthz loopback) y espera el
// resultado (tarda minutos: clone+build). Token en env del servicio.
func cmdDeploy(args []string, cfgPath string) {
	if len(args) != 1 || args[0] == "" {
		log.Fatal("uso: deploy <app>")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("deploy: %v", err)
	}
	found := false
	for i := range cfg.Apps {
		if cfg.Apps[i].Name == args[0] {
			found = true
			if cfg.Apps[i].Deploy == nil {
				log.Fatalf("deploy: app %q sin bloque deploy en config", args[0])
			}
		}
	}
	if !found {
		log.Fatalf("deploy: app %q inexistente en config", args[0])
	}
	if !cfg.Healthz.Enabled {
		log.Fatal("deploy: requiere healthz activado")
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Post("http://"+cfg.Healthz.Listen+"/deploy/"+args[0], "application/json", nil)
	if err != nil {
		log.Fatalf("deploy: sin contacto con el servicio: %v", err)
	}
	defer resp.Body.Close()
	var out struct {
		Deployed string `json:"deployed"`
		Rev      string `json:"rev"`
		Error    string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("deploy: %s", out.Error)
	}
	fmt.Printf("deploy ok: %s rev %s\n", out.Deployed, out.Rev)
}

// cmdLogs imprime últimas N líneas del log y opcionalmente sigue (-f).
// Con -app NOMBRE lee logs/apps/NOMBRE.log (sin follow).
func cmdLogs(args []string, cfgPath string) {
	n := 50
	follow := false
	appName := ""
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
		case "-app":
			i++
			if i >= len(args) {
				log.Fatal("logs: -app requiere nombre")
			}
			appName = args[i]
		default:
			log.Fatalf("logs: flag desconocido %q (uso: logs [-n 50] [-f] [-app nombre])", args[i])
		}
	}
	path := logx.Path(cfgPath)
	if appName != "" {
		path = filepath.Join(logx.DirFor(cfgPath), "apps", appName+".log")
		follow = false
	}
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
