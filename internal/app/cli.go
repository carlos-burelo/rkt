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

	"rkt-proxy/internal/build"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/mcpserver"
	"rkt-proxy/internal/staticweb"
	"rkt-proxy/internal/store"
)

// JSON salida máquina para comandos de lectura (--json).
var JSON = flag.Bool("json", false, "salida JSON en comandos de lectura")

// deployTimeout POSTs largos al servicio (deploy/rollback tardan minutos).
const deployTimeout = 10 * time.Minute

// Run cablea logger (consola+archivo), servicio Windows y despacha.
// Sin args: ayuda si interactivo, servicio si no (arranque por SCM).
func Run(cfgPath string) {
	logger, closeLog, err := logx.Setup(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	defer closeLog()

	svcConfig := &service.Config{
		Name:        "Rkt",
		DisplayName: "RKT",
		Description: "PaaS Go: dominios con TLS auto, apps supervisadas y deploys.",
		Arguments:   []string{"-config", logx.Abs(cfgPath)},
		Option: service.KeyValue{
			// arranca solo tras instalar + en cada boot (sin comandos)
			"StartImmediately": true,
		},
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
			fmt.Println("servicio instalado y arrancado.")
			printDashboardURL(cfgPath)
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
			RunForeground(cfgPath)
			return
		case "__static":
			// interno: el supervisor arranca sitios estáticos así
			if len(flag.Args()) < 2 {
				log.Fatal("uso: rkt __static <dir>")
			}
			if err := staticweb.Serve(flag.Arg(1)); err != nil {
				log.Fatalf("static: %v", err)
			}
			return
		case "version":
			fmt.Println(build.Version)
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
		case "users":
			cmdUsers(flag.Args()[1:], cfgPath)
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
			cmdStatus(flag.Args()[1:], cfgPath)
			return
		case "logs":
			cmdLogs(flag.Args()[1:], cfgPath)
			return
		case "dev":
			cmdDev(flag.Args()[1:], cfgPath)
			return
		case "mcp":
			runMCP(cfgPath)
			return
		case "hosts":
			cmdHosts(flag.Args()[1:])
			return
		case "help", "-h", "--help":
			usage(0)
			return
		default:
			fmt.Fprintf(os.Stderr, "comando desconocido %q\n\n", flag.Arg(0))
			usage(2)
			return
		}
	}

	if service.Interactive() {
		usage(0)
		return
	}
	if err := s.Run(); err != nil {
		logger.Error("servicio muerto", "err", err)
		os.Exit(1)
	}
}

// runMCP sirve el servidor MCP por stdio (para agentes). Bloquea.
func runMCP(cfgPath string) {
	srv := mcpserver.New(cfgPath)
	if err := srv.Run(context.Background()); err != nil {
		log.Fatalf("mcp: %v", err)
	}
}

// usage imprime la referencia CLI y sale con code.
// Códigos: 0 ok, 1 error runtime, 2 mal uso.
func usage(code int) {
	out := os.Stdout
	if code != 0 {
		out = os.Stderr
	}
	fmt.Fprintln(out, `rkt [-config path] [--json] <comando>

Servicio:
  install | uninstall | start | stop     servicio Windows
  run                                    foreground (Ctrl+C limpio)
  dev [--http 127.0.0.1:80] [--https 127.0.0.1:443]
                                         foreground local con puertos de prod
                                         y TLS de CA propia (CA en logs/devca)
  mcp                                    servidor MCP stdio (para agentes)
  hosts list|add|rm <host>               DNS local (hosts del sistema, admin)
  version                                versión del binario

Operate (contra el servicio en marcha):
  status [--json]                        salud + apps + backends
  logs [-n 50] [-f] [-app nombre]        logs (operativo o de app)
  restart <app>                          reinicio supervisado inmediato
  deploy <app>                           pull+build+swap+restart+rollback
  rollback <app>                         vuelve a live.prev
  deployments [-n 20] [--app x] [--json] historial SQLite

Gestión (flota en SQLite, con backup implícito):
  apps list|add|rm                       --name --command [--preset next|node|python] [--repo --domain --port --auto]
  routes list|add|rm                     --host (repetible) --target
  env list|set|unset <app> [CLAVE[=valor]] env por app
  env global [set CLAVE=valor|unset CLAVE]  variables de toda la flota
  settings list|set                      settings set CLAVE=valor
  users list|add|del|passwd              usuarios dashboard (primer add = admin)
  config export [file]|import <file>     respaldo/restaura en git
  github login|init                      vincular GitHub App en browser

Ayuda:
  help                                   esta pantalla`)
	os.Exit(code)
}

// printDashboardURL dice dónde entra el operador (sin comandos extra:
// el servicio ya arrancó y el setup inicial vive en la propia UI).
func printDashboardURL(cfgPath string) {
	cfg, err := cfgfile.Load(cfgPath)
	if err != nil {
		return
	}
	if !cfg.Healthz.Enabled {
		fmt.Println("aviso: healthz desactivado, sin dashboard local")
		return
	}
	fmt.Printf("dashboard: http://%s/ (crea tu admin ahí la primera vez)\n", cfg.Healthz.Listen)
}

// cmdDev levanta entorno local con los mismos puertos que producción
// (80/443) y TLS con CA propia, para que las URLs no cambien entre entornos.
func cmdDev(args []string, cfgPath string) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	httpAddr := fs.String("http", "127.0.0.1:80", "dirección HTTP local")
	httpsAddr := fs.String("https", "127.0.0.1:443", "dirección HTTPS local con CA propia (vacío = sin TLS)")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *httpAddr == "" {
		log.Fatal("dev: --http no puede estar vacío")
	}
	path := cfgPath
	if path == "" || path == "rkt.db" {
		path = "rkt.dev.db"
	}
	db, err := store.Open(logx.DBPath(path))
	if err != nil {
		log.Fatalf("dev: %v", err)
	}
	if _, statErr := os.Stat(logx.Abs(path)); statErr == nil {
		// migra YAML existente a DB (una vez) sin perder infra
		if _, err := cfgfile.LoadBase(db, path); err != nil {
			log.Fatalf("dev: migrar config: %v", err)
		}
	}
	if err := cfgfile.ApplyDev(db, *httpAddr, *httpsAddr); err != nil {
		log.Fatalf("dev: %v", err)
	}
	_ = db.Close()
	fmt.Println("Datos en", logx.DirFor(path), "(config en SQLite, sin YAML)")
	fmt.Println("Dominios locales: usa *.localhost (resuelve solo) o `rkt hosts add <host>` como admin.")
	fmt.Printf("HTTP %s · HTTPS %s\n", *httpAddr, *httpsAddr)
	fmt.Println("TLS de desarrollo: CA propia en logs/devca/ca.crt (se instala en Root del usuario).")
	RunForeground(path)
}

// RunForeground corre el programa en foreground con Ctrl+C limpio.
func RunForeground(cfgPath string) {
	logger, closeLog, err := logx.Setup(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	defer closeLog()
	prg := New(cfgPath, logger)
	ctx, sigstop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer sigstop()
	go func() {
		<-ctx.Done()
		_ = prg.Stop(nil)
		os.Exit(0)
	}()
	prg.Run()
}

// cmdRestart pide al servicio (vía healthz loopback) reinicio sin cortes:
// rkt-proxy restart <app>
func cmdRestart(args []string, cfgPath string) {
	if len(args) != 1 || args[0] == "" {
		log.Fatal("uso: restart <app>")
	}
	cfg, err := cfgfile.Load(cfgPath)
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
	cfg, err := cfgfile.Load(cfgPath)
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
	client := &http.Client{Timeout: deployTimeout}
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
	out, err := logx.Tail(path, n)
	if err != nil {
		log.Fatalf("logs: %v", err)
	}
	for _, l := range strings.Split(out, "\n") {
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
