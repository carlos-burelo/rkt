package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kardianos/service"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/proxy"
)

var (
	configPath = flag.String("config", "config.yaml", "ruta config YAML")
)

type program struct {
	cfgPath string
	log     *slog.Logger
	srvHTTP *http.Server
	srvTLS  *http.Server
	mgr     *autocert.Manager
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) run() {
	cfg, err := config.Load(p.cfgPath)
	if err != nil {
		p.log.Error("config inválida", "err", err)
		os.Exit(1)
	}

	handler := proxy.NewHandler(cfg, p.log)

	allowed := map[string]bool{}
	for _, h := range cfg.AllowedHosts() {
		allowed[h] = true
	}
	hostPolicy := func(ctx context.Context, host string) error {
		host = strings.ToLower(strings.TrimSpace(host))
		if allowed[host] {
			return nil
		}
		// permite subdominio de base registrada (para *.dominio)
		for base := range allowed {
			if strings.HasSuffix(host, "."+base) {
				return nil
			}
		}
		return fmt.Errorf("host %q no permitido", host)
	}

	dirCache := autocert.DirCache(cfg.ACMECacheDir)
	if err := os.MkdirAll(cfg.ACMECacheDir, 0o700); err != nil {
		p.log.Warn("no se pudo crear cache dir", "err", err)
	}
	p.mgr = &autocert.Manager{
		Cache:      dirCache,
		Prompt:     autocert.AcceptTOS,
		Email:      cfg.ACMEEmail,
		HostPolicy: hostPolicy,
	}
	if cfg.ACMEStaging {
		p.mgr.Client = &acme.Client{DirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"}
		p.log.Warn("usando Let's Encrypt STAGING")
	}

	// HTTP: challenge ACME + redirect a HTTPS
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			p.mgr.HTTPHandler(nil).ServeHTTP(w, r)
			return
		}
		if cfg.RedirectHTTPToHTTPS {
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		handler.ServeHTTP(w, r)
	})
	p.srvHTTP = &http.Server{
		Addr:         cfg.ListenHTTP,
		Handler:      httpMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// HTTPS: TLS con certs auto
	p.srvTLS = &http.Server{
		Addr:         cfg.ListenHTTPS,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: p.mgr.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		},
	}

	p.log.Info("arrancando",
		"http", cfg.ListenHTTP,
		"https", cfg.ListenHTTPS,
		"routes", len(cfg.Routes),
		"email", cfg.ACMEEmail,
	)

	go func() {
		if err := p.srvHTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.log.Error("http muerto", "err", err)
		}
	}()
	if err := p.srvTLS.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		p.log.Error("https muerto", "err", err)
		os.Exit(1)
	}
}

func (p *program) Stop(s service.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.srvTLS != nil {
		_ = p.srvTLS.Shutdown(ctx)
	}
	if p.srvHTTP != nil {
		_ = p.srvHTTP.Shutdown(ctx)
	}
	return nil
}

func main() {
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	svcConfig := &service.Config{
		Name:        "RktProxy",
		DisplayName: "RKT Reverse Proxy",
		Description: "Reverse proxy Go: dominios -> puertos locales, TLS Let's Encrypt auto.",
		Arguments:   []string{"-config", absoluteConfigPath(*configPath)},
	}
	prg := &program{cfgPath: *configPath, log: logger}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) > 1 {
		cmd := os.Args[1]
		switch cmd {
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
			prg.run()
			return
		}
	}

	// Sin args: si corre como servicio Windows lo detecta, si no corre foreground.
	if service.Interactive() {
		fmt.Println("modo interactivo: usa 'run' para foreground o install/start como servicio")
		prg.run()
		return
	}
	if err := s.Run(); err != nil {
		logger.Error("servicio muerto", "err", err)
		os.Exit(1)
	}
}

func absoluteConfigPath(p string) string {
	if len(p) > 1 && (p[1] == ':' || p[0] == '/' || p[0] == '\\') {
		return p
	}
	// servicio Windows arranca en System32: resolver a dir del exe
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
