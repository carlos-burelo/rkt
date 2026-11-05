package api

import (
	"net/http"

	"rkt-proxy/internal/health"
)

// handleStatus salud resumida + puertos públicos + apps.
func (d *Deps) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	apps := []health.AppStatus{}
	if d.Apps != nil {
		apps = d.Apps()
	}
	cfg := d.Live.Get()
	httpPort := listenPort(cfg.ListenHTTP, "80")
	httpsPort := ""
	if cfg.ListenHTTPS != "" {
		httpsPort = listenPort(cfg.ListenHTTPS, "443")
	}
	writeJSON(w, map[string]any{
		"version":    d.Version,
		"routes":     len(cfg.Routes),
		"apps":       apps,
		"http_port":  httpPort,
		"https_port": httpsPort,
	})
}
