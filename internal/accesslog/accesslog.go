// Package accesslog: access log estilo combinado a archivo,
// separado del log operativo. Para forense de escaneos.
package accesslog

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxBytes = 10 << 20 // 10MB, luego rota a .1

// Logger escribe líneas con lock (concurrente).
type Logger struct {
	mu sync.Mutex
	f  *os.File
}

// New abre (creando dir) el archivo en append.
func New(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("crear access dir: %w", err)
	}
	if st, err := os.Stat(path); err == nil && st.Size() > maxBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("abrir access log: %w", err)
	}
	return &Logger{f: f}, nil
}

// Close cierra el archivo.
func (l *Logger) Close() error { return l.f.Close() }

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *recorder) WriteHeader(s int) {
	r.status = s
	r.ResponseWriter.WriteHeader(s)
}

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
		l.mu.Lock()
		fmt.Fprintf(l.f, "%s - - [%s] %q %d %d %q %q host=%q dur=%s\n",
			r.RemoteAddr,
			start.Format("02/Jan/2006:15:04:05 -0700"),
			r.Method+" "+r.URL.RequestURI()+" "+r.Proto,
			rec.status, rec.bytes,
			r.Referer(), r.UserAgent(),
			r.Host, time.Since(start).Truncate(time.Microsecond),
		)
		l.mu.Unlock()
	})
}
