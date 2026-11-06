package logx

import (
	"fmt"
	"os"
	"sync"
)

// RotatingFile escribe con rotación por tamaño en caliente. Conserva un
// único backup .1 (mismo contrato que el arranque) y es seguro concurrente.
type RotatingFile struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

// NewRotating abre (creando dir) y rota si el archivo ya supera max.
func NewRotating(path string, max int64) (*RotatingFile, error) {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 10 << 20
	}
	r := &RotatingFile{path: path, max: max}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

func (r *RotatingFile) open() error {
	if st, err := os.Stat(r.path); err == nil {
		if st.IsDir() {
			return fmt.Errorf("log path es un directorio: %s", r.path)
		}
		if st.Size() > r.max {
			_ = os.Rename(r.path, r.path+".1")
		} else {
			r.size = st.Size()
		}
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	r.f = f
	if st, err := f.Stat(); err == nil {
		r.size = st.Size()
	}
	return nil
}

// Write rota antes de escribir si el bloque no cabe.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, fmt.Errorf("log cerrado")
	}
	if r.size+int64(len(p)) > r.max {
		_ = r.f.Close()
		_ = os.Rename(r.path, r.path+".1")
		r.size = 0
		f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return 0, err
		}
		r.f = f
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// Close cierra el archivo.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
