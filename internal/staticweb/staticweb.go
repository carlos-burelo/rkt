// Package staticweb sirve sitios estáticos (Astro, Vite, export) con el
// propio binario, sin proceso extra de Node. Se ejecuta como app hija del
// supervisor: `rkt __static <dir>` escuchando en $PORT.
package staticweb

import (
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Tipos que el registro de Windows no siempre conoce (sitios modernos).
func init() {
	for ext, typ := range map[string]string{
		".avif":  "image/avif",
		".webp":  "image/webp",
		".woff2": "font/woff2",
		".wasm":  "application/wasm",
		".mp4":   "video/mp4",
		".webm":  "video/webm",
		".svg":   "image/svg+xml",
		".json":  "application/json",
	} {
		_ = mime.AddExtensionType(ext, typ)
	}
}

// cacheFor: assets con hash son inmutables; html se revalida siempre.
func cacheFor(clean, ext string) string {
	if strings.HasPrefix(clean, "/_astro/") || strings.HasPrefix(clean, "/assets/") ||
		strings.HasPrefix(clean, "/static/") || strings.HasPrefix(clean, "/_next/static/") {
		return "public, max-age=31536000, immutable"
	}
	if ext == "" || ext == ".html" {
		return "no-cache"
	}
	return "public, max-age=3600"
}

// Handler sirve root con fallback: 404.html si existe (404) o index.html
// (SPA). Así sirve tanto sitios multipágina como SPAs sin configuración.
func Handler(root string) http.Handler {
	fsrv := http.FileServer(http.Dir(root))
	index := filepath.Join(root, "index.html")
	notFound := filepath.Join(root, "404.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		target := filepath.Join(root, filepath.FromSlash(clean))
		ext := strings.ToLower(filepath.Ext(clean))
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			w.Header().Set("Cache-Control", cacheFor(clean, ext))
			fsrv.ServeHTTP(w, r)
			return
		}
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(target, "index.html")); err == nil {
				w.Header().Set("Cache-Control", "no-cache")
				fsrv.ServeHTTP(w, r)
				return
			}
		}
		// Un asset que no existe NO debe devolver el index (el navegador
		// recibiría HTML como JS/CSS). Solo rutas sin extensión son SPA.
		if ext != "" && ext != ".html" {
			w.Header().Set("Cache-Control", "no-cache")
			if _, err := os.Stat(notFound); err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				http.ServeFile(w, r, notFound)
				return
			}
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(notFound); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusNotFound)
			http.ServeFile(w, r, notFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

// validateRoot exige sitio compilado: mejor fallar que servir 200 vacío.
func validateRoot(root string) error {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("no existe el directorio %s (compila primero)", root)
	}
	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		return fmt.Errorf("%s sin index.html (build incompleto)", root)
	}
	return nil
}

// listen es net.Listen (inyectable en tests para no abrir puertos fijos).
var listen = net.Listen

// Serve arranca el servidor en $PORT (el supervisor lo inyecta).
// Falla si no hay sitio compilado: mejor unidad failed que 200 mentiroso.
func Serve(root string) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	if err := validateRoot(root); err != nil {
		return err
	}
	ln, err := listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("escuchar :%s: %w", port, err)
	}
	defer ln.Close()
	return ServeOn(ln, root)
}

// ServeOn sirve sobre un listener ya abierto (testeable sin puerto fijo).
func ServeOn(ln net.Listener, root string) error {
	if err := validateRoot(root); err != nil {
		return err
	}
	log.Printf("static %s en %s", root, ln.Addr())
	return http.Serve(ln, Handler(root))
}
