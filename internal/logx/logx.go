// Package logx: logger a consola + archivo. El servicio Windows
// no tiene stdout visible, el archivo es el único log real.
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// maxBytes rota rkt-proxy.log a .1 al arrancar si se supera.
const maxBytes = 10 << 20 // 10MB

// Abs resuelve un path relativo junto al .exe (el servicio
// Windows arranca en System32 y la ruta relativa rompería).
func Abs(p string) string {
	if len(p) > 1 && (p[1] == ':' || p[0] == '/' || p[0] == '\\') {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return p
	}
	return filepath.Join(filepath.Dir(exe), p)
}

// DirFor devuelve <dir-config>/logs para cualquier config path.
func DirFor(configPath string) string {
	return filepath.Join(filepath.Dir(Abs(configPath)), "logs")
}

// Path ruta del log para el subcomando `logs`.
func Path(configPath string) string {
	return filepath.Join(DirFor(configPath), "rkt-proxy.log")
}

// AccessPath ruta del access log.
func AccessPath(configPath string) string {
	return filepath.Join(DirFor(configPath), "access.log")
}

// DBPath ruta de rkt.db (historial SQLite).
func DBPath(configPath string) string {
	return filepath.Join(DirFor(configPath), "rkt.db")
}

// Setup abre (creando dir) el log y devuelve logger multi
// consola+archivo más closer.
func Setup(configPath string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(DirFor(configPath), 0o755); err != nil {
		return nil, nil, fmt.Errorf("crear log dir: %w", err)
	}
	path := Path(configPath)
	if st, err := os.Stat(path); err == nil && st.Size() > maxBytes {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("abrir log: %w", err)
	}
	// Archivo primero: como servicio no hay consola y el write a
	// stdout falla; MultiWriter aborta al primer error y el log
	// quedaría vacío. El error de stdout lo ignora slog igual.
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(f, os.Stdout), nil))
	return logger, func() { _ = f.Close() }, nil
}
