// Package logx: logger a consola + archivo. Como servicio no hay stdout
// visible, el archivo es el único log real.
package logx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxBytes rota rkt.log a .1 al arrancar si se supera.
const maxBytes = 10 << 20 // 10MB

// Abs resuelve un path relativo de forma determinista: primero junto al
// CWD (dev desde el repo) y, si no existe, junto al .exe (servicio del
// sistema, que arranca en otro CWD). Un mismo -config no debe cambiar la carpeta
// de datos según cómo se escriba.
// getwd/executable inyectables para tests de rutas.
var (
	getwd      = os.Getwd
	executable = os.Executable
)

func Abs(p string) string {
	if len(p) > 1 && (p[1] == ':' || p[0] == '/' || p[0] == '\\') {
		return p
	}
	if wd, err := getwd(); err == nil {
		if c := filepath.Join(wd, p); fileExists(c) {
			return c
		}
	}
	if exe, err := executable(); err == nil {
		if c := filepath.Join(filepath.Dir(exe), p); fileExists(c) {
			return c
		}
	}
	// Sin archivo: el CWD es el ancla estable (dev desde el repo).
	if wd, err := getwd(); err == nil {
		return filepath.Join(wd, p)
	}
	return p
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// DirFor devuelve <dir-config>/logs para cualquier config path.
func DirFor(configPath string) string {
	return filepath.Join(filepath.Dir(Abs(configPath)), "logs")
}

// Path ruta del log para el subcomando `logs`.
func Path(configPath string) string {
	return filepath.Join(DirFor(configPath), "rkt.log")
}

// AccessPath ruta del access log.
func AccessPath(configPath string) string {
	return filepath.Join(DirFor(configPath), "access.log")
}

// DBPath ruta de rkt.db (historial SQLite).
func DBPath(configPath string) string {
	return filepath.Join(DirFor(configPath), "rkt.db")
}

// AppKeyPath ruta del PEM de la GitHub App (lo escribe el setup).
func AppKeyPath(configPath string) string {
	return filepath.Join(DirFor(configPath), "github-app.pem")
}

// ControlTokenPath ruta del token de control loopback (acciones admin).
func ControlTokenPath(configPath string) string {
	return filepath.Join(DirFor(configPath), "control.token")
}

// EnsureControlToken lee o crea el token de control con ACL del directorio
// de logs. Nunca se sirve por API: CLI y servicio comparten el archivo.
func EnsureControlToken(configPath string) (string, error) {
	path := ControlTokenPath(configPath)
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) >= 32 {
			return t, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("token de control: %w", err)
	}
	tok := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(tok+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("token de control: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return tok, nil
}

// Tail últimas n líneas de un archivo. n<1 = 50. Canónica; CLI y MCP
// la usan (antes 3 copias del Split/TrimRight).
func Tail(path string, n int) (string, error) {
	if n < 1 {
		n = 50
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return StripANSI(strings.Join(lines, "\n")), nil
}

// ansiRx secuencias de color/control de terminal (build logs de pnpm,
// astro, go…). El dashboard no las renderiza: se limpian al servir.
var ansiRx = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// StripANSI quita códigos ANSI conservando el texto.
func StripANSI(s string) string { return ansiRx.ReplaceAllString(s, "") }

// Setup abre (creando dir) el log y devuelve logger multi
// consola+archivo más closer. Rota en caliente al pasar maxBytes.
func Setup(configPath string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(DirFor(configPath), 0o755); err != nil {
		return nil, nil, fmt.Errorf("crear log dir: %w", err)
	}
	f, err := NewRotating(Path(configPath), maxBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("abrir log: %w", err)
	}
	// Archivo primero: como servicio no hay consola y el write a
	// stdout falla; MultiWriter aborta al primer error y el log
	// quedaría vacío. El error de stdout lo ignora slog igual.
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(f, os.Stdout), nil))
	return logger, func() { _ = f.Close() }, nil
}
