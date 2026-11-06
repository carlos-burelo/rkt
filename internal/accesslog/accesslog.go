// Package accesslog: access log estilo combinado a archivo,
// separado del log operativo. Para forense de escaneos.
package accesslog

import (
	"fmt"
	"net/http"
	"time"

	"rkt-proxy/internal/logx"
)

const maxBytes = 10 << 20 // 10MB, luego rota a .1

// Logger escribe líneas con rotación en caliente y lock interno.
type Logger struct {
	f *logx.RotatingFile
}

// New abre (creando dir) el archivo en append.
func New(path string) (*Logger, error) {
	f, err := logx.NewRotating(path, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("abrir access log: %w", err)
	}
	return &Logger{f: f}, nil
}

// Close cierra el archivo.
func (l *Logger) Close() error { return l.f.Close() }

type recorder struct {
	http.ResponseWriter
	status int // código final (>=200) ya enviado
	bytes  int
}

// WriteHeader ignora cabeceras finales repetidas: si un handler ya respondió y
// luego algo (recovery, error tardío) intenta escribir, net/http solo soltaría
// "superfluous response.WriteHeader call". Los 1xx (Early Hints) sí pasan.
func (r *recorder) WriteHeader(s int) {
	if s >= 200 {
		if r.status != 0 {
			return
		}
		r.status = s
	}
	r.ResponseWriter.WriteHeader(s)
}

// Unwrap deja que http.ResponseController encuentre Flusher/Hijacker/ReaderFrom
// del writer real: sin esto se rompen streaming, SSE y rangos de video.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Middleware registra cada respuesta en formato combinado + extras.
func (l *Logger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		fmt.Fprintf(l.f, "%s - - [%s] %q %d %d %q %q host=%q req=%q dur=%s\n",
			r.RemoteAddr,
			start.Format("02/Jan/2006:15:04:05 -0700"),
			r.Method+" "+r.URL.RequestURI()+" "+r.Proto,
			rec.status, rec.bytes,
			r.Referer(), r.UserAgent(),
			r.Host, r.Header.Get("X-Request-Id"), time.Since(start).Truncate(time.Microsecond),
		)
	})
}
