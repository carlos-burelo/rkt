// Package server construye los servidores HTTP (:80, challenge ACME
// + redirect) y HTTPS (:443, certs auto). Sin lógica de negocio.
package server

import (
	"crypto/tls"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/config"
)

// NewHTTP sirve el challenge ACME en HTTP y redirige el resto a
// HTTPS (o proxea directo si redirect desactivado).
func NewHTTP(cfg *config.Config, mgr *autocert.Manager, backend http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			mgr.HTTPHandler(nil).ServeHTTP(w, r)
			return
		}
		if cfg.RedirectHTTPToHTTPS {
			http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		backend.ServeHTTP(w, r)
	})
	return &http.Server{
		Addr:         cfg.ListenHTTP,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// NewHTTPS sirve el backend con TLS y certs del Manager.
func NewHTTPS(cfg *config.Config, mgr *autocert.Manager, backend http.Handler) *http.Server {
	return &http.Server{
		Addr:         cfg.ListenHTTPS,
		Handler:      backend,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig: &tls.Config{
			GetCertificate: mgr.GetCertificate,
			MinVersion:     tls.VersionTLS12,
		},
	}
}
