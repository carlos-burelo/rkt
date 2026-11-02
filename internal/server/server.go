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
	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/config"
)

// NewHTTP sirve el challenge ACME en HTTP y redirige el resto a
// HTTPS (o proxea directo si redirect desactivado).
// challenge debe construirse una vez con mgr.HTTPHandler(nil): eso
// además activa http-01 como fallback en el Manager.
// El flag redirect se lee de la config viva (hot-reload).
func NewHTTP(cfg *config.Config, live *config.Live, challenge http.Handler, backend http.Handler, errLog *log.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			challenge.ServeHTTP(w, r)
			return
		}
		if live.Get().RedirectHTTPToHTTPS {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		backend.ServeHTTP(w, r)
	})
	return &http.Server{
		Addr:         cfg.ListenHTTP,
		Handler:      mux,
		ErrorLog:     errLog,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// NewHTTPS sirve el backend con TLS y certs del Manager.
// NextProtos incluye acme.ALPNProto: sin él Go rechaza el handshake
// tls-alpn-01 antes de que GetCertificate pueda responder.
func NewHTTPS(cfg *config.Config, mgr *autocert.Manager, backend http.Handler, errLog *log.Logger) *http.Server {
	return &http.Server{
		Addr:         cfg.ListenHTTPS,
		Handler:      backend,
		ErrorLog:     errLog,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: mgr.GetCertificate,
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{"h2", "http/1.1", acme.ALPNProto},
		},
	}
}
