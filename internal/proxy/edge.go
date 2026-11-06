package proxy

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

// RequestID propaga X-Request-Id (generado si falta) y lo expone en la
// cabecera del request para que el log y el backend lo compartan.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if id == "" || len(id) > 64 {
			raw := make([]byte, 8)
			_, _ = rand.Read(raw)
			id = hex.EncodeToString(raw)
		}
		w.Header().Set("X-Request-Id", id)
		r.Header.Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

// EdgeHeaders añade cabeceras de seguridad que no rompen apps arbitrarias.
// HSTS solo cuando el request llegó por TLS.
func EdgeHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// Compress aplica gzip a respuestas compresibles. Nunca toca upgrades,
// SSE, rangos ni respuestas ya codificadas.
func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponse{ResponseWriter: w}
		defer gw.Close()
		next.ServeHTTP(gw, r)
	})
}

type gzipResponse struct {
	http.ResponseWriter
	gz       *gzip.Writer
	decided  bool
	hijacked bool
}

func compressible(ct string, status int) bool {
	if status == http.StatusNoContent || status == http.StatusNotModified ||
		status == http.StatusPartialContent {
		return false
	}
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch {
	case strings.HasPrefix(ct, "text/"),
		ct == "application/json",
		ct == "application/javascript",
		ct == "application/xml",
		ct == "application/xhtml+xml",
		ct == "image/svg+xml":
		return true
	}
	return false
}

func (g *gzipResponse) decide(status int) {
	if g.decided {
		return
	}
	g.decided = true
	h := g.Header()
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" {
		return
	}
	if !compressible(h.Get("Content-Type"), status) {
		return
	}
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	g.gz = gzip.NewWriter(g.ResponseWriter)
}

func (g *gzipResponse) WriteHeader(status int) {
	g.decide(status)
	g.ResponseWriter.WriteHeader(status)
}

func (g *gzipResponse) Write(p []byte) (int, error) {
	if !g.decided {
		g.decide(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipResponse) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponse) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipResponse) Close() {
	if g.gz != nil {
		_ = g.gz.Close()
	}
}
