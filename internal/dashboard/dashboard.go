// Package dashboard sirve la SPA embebida (web/dist) en la raíz del
// listener interno. El HTML/JS es público a propósito: el wizard inicial
// (sin usuarios) y el login viven en la SPA; los DATOS van tras token en
// /api/v1. Autenticar el HTML impediría el primer arranque (bug real).
package dashboard

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"time"
)

func init() {
	// ServeContent no conoce .webmanifest; sin esto el manifest sale
	// como text/plain (algunos navegadores lo rechazan).
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// cacheFor: bundles con hash inmutables; shell, sw y manifest siempre
// revalidan para que un deploy se vea al recargar (sin Ctrl+Shift+R).
func cacheFor(p string) string {
	if strings.HasPrefix(p, "assets/") || strings.HasPrefix(p, "workbox-") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// NewHandler SPA en "/" con fallback a index.html (history routing).
// fsys = web/dist (embed en prod, DirFS en tests).
func NewHandler(fsys fs.FS) http.Handler {
	fileSrv := http.FileServer(http.FS(fsys))
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("dashboard sin compilar: corre `pnpm --dir web build`"))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(data))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			serveIndex(w, r)
			return
		}
		// FileServer redirige index.html a ./ por diseño: nunca cae aquí
		// porque "" se sirve arriba; resto son assets o fallback SPA.
		if _, err := fs.Stat(fsys, path); err != nil {
			serveIndex(w, r)
			return
		}
		w.Header().Set("Cache-Control", cacheFor(path))
		if path == "sw.js" {
			w.Header().Set("Service-Worker-Allowed", "/")
		}
		r.URL.Path = "/" + path
		fileSrv.ServeHTTP(w, r)
	})
}
