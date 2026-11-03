// Package dashboard sirve la SPA embebida (web/dist) en /dashboard/.
// El HTML/JS es público a propósito: el wizard inicial (sin usuarios)
// y el login viven en la SPA; los DATOS van tras token en /api/v1.
// Autenticar el HTML impediría el primer arranque (bug real).
package dashboard

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// NewHandler SPA con fallback a index.html (history routing).
// fsys = web/dist (embed en prod, DirFS en tests).
func NewHandler(fsys fs.FS) http.Handler {
	fileSrv := http.FileServer(http.FS(fsys))
	mux := http.NewServeMux()
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("dashboard sin compilar: corre `pnpm --dir web build`"))
			return
		}
		http.ServeContent(w, r, "index.html", time.Now(), bytes.NewReader(data))
	}
	mux.HandleFunc("/dashboard/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/dashboard/")
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
		r.URL.Path = "/" + path
		fileSrv.ServeHTTP(w, r)
	})
	return mux
}
