// Package staticweb sirve sitios estáticos (Astro, Vite, export) con el
// propio binario, sin proceso extra de Node. Se ejecuta como app hija del
// supervisor: `rkt __static <dir>` escuchando en $PORT.
package staticweb

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Handler sirve root con fallback: 404.html si existe (404) o index.html
// (SPA). Así sirve tanto sitios multipágina como SPAs sin configuración.
func Handler(root string) http.Handler {
	fsrv := http.FileServer(http.Dir(root))
	index := filepath.Join(root, "index.html")
	notFound := filepath.Join(root, "404.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		target := filepath.Join(root, filepath.FromSlash(clean))
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			fsrv.ServeHTTP(w, r)
			return
		}
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(target, "index.html")); err == nil {
				fsrv.ServeHTTP(w, r)
				return
			}
		}
		if _, err := os.Stat(notFound); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			http.ServeFile(w, r, notFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFile(w, r, index)
	})
}

// Serve arranca el servidor en $PORT (el supervisor lo inyecta).
func Serve(root string) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		log.Fatalf("static: no existe el directorio %s", root)
	}
	log.Printf("static %s en :%s", root, port)
	return http.ListenAndServe(":"+port, Handler(root))
}
