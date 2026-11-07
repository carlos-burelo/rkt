// Package server construye los servidores HTTP (:80, challenge ACME
// + redirect) y HTTPS (:443, certs auto). Sin lógica de negocio.
package server

import (
	"crypto/tls"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/acme"

	"rkt-proxy/internal/config"
)

// NewHTTP sirve el challenge ACME en HTTP y redirige el resto a
// HTTPS (o proxea directo si redirect desactivado).
// challenge debe construirse una vez con mgr.HTTPHandler(nil): eso
// además activa http-01 como fallback en el Manager.
// control (puede ser nil) atiende /_rkt/* antes que todo (plano control).
// El flag redirect se lee de la config viva (hot-reload).
func NewHTTP(cfg *config.Config, live *config.Live, challenge http.Handler, backend http.Handler, control http.Handler, errLog *log.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_rkt/") {
			if control == nil {
				http.NotFound(w, r)
				return
			}
			control.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			if challenge == nil {
				http.NotFound(w, r)
				return
			}
			challenge.ServeHTTP(w, r)
			return
		}
		cfg := live.Get()
		if cfg.RedirectHTTPToHTTPS && (cfg.ListenHTTPS != "" || cfg.LocalCA) {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		backend.ServeHTTP(w, r)
	})
	return &http.Server{
		Addr:     cfg.ListenHTTP,
		Handler:  mux,
		ErrorLog: errLog,
		// Solo cabeceras: ReadTimeout acota el body completo y rompe
		// uploads lentos. WriteTimeout 0: SSE/vídeo pueden durar más de
		// 60s; los límites se aplican por ruta en el proxy.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// NewHTTPS sirve el backend con TLS. getCert elige el certificado en cada
// handshake: autocert (producción) o CA local de desarrollo.
// NextProtos incluye acme.ALPNProto: sin él Go rechaza el handshake
// tls-alpn-01 antes de que GetCertificate pueda responder.
// control (puede ser nil) atiende /_rkt/* antes que el backend.
func NewHTTPS(cfg *config.Config, getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error), backend http.Handler, control http.Handler, errLog *log.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_rkt/") {
			if control == nil {
				http.NotFound(w, r)
				return
			}
			control.ServeHTTP(w, r)
			return
		}
		backend.ServeHTTP(w, r)
	})
	return &http.Server{
		Addr:              cfg.ListenHTTPS,
		Handler:           mux,
		ErrorLog:          errLog,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: getCert,
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{"h2", "http/1.1", acme.ALPNProto},
		},
	}
}
