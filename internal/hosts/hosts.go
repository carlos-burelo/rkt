// Package hosts gestiona el bloque de rkt en el archivo hosts del sistema.
// Solo toca un tramo marcado: no reescribe entradas ajenas. Escribir exige
// privilegios de administrador (el archivo pertenece al sistema).
package hosts

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	beginMark = "# rkt-managed begin (no editar a mano)"
	endMark   = "# rkt-managed end"
)

// Path ruta del hosts (RKT_HOSTS_PATH permite tests sin tocar el sistema).
func Path() string {
	if p := os.Getenv("RKT_HOSTS_PATH"); p != "" {
		return p
	}
	if runtime.GOOS != "windows" {
		return "/etc/hosts"
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// List hosts gestionados por rkt.
func List() ([]string, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, line := range managedLines(string(data)) {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "127.0.0.1" {
			out = append(out, f[1])
		}
	}
	sort.Strings(out)
	return out, nil
}

// Add asegura "<host> -> 127.0.0.1" dentro del bloque de rkt.
func Add(host string) error {
	h, err := normalize(host)
	if err != nil {
		return err
	}
	return mutate(func(managed []string) []string {
		for _, l := range managed {
			if f := strings.Fields(l); len(f) >= 2 && strings.EqualFold(f[1], h) {
				return managed
			}
		}
		return append(managed, "127.0.0.1\t"+h)
	})
}

// Remove quita un host del bloque de rkt.
func Remove(host string) error {
	h, err := normalize(host)
	if err != nil {
		return err
	}
	return mutate(func(managed []string) []string {
		kept := managed[:0]
		for _, l := range managed {
			if f := strings.Fields(l); len(f) >= 2 && strings.EqualFold(f[1], h) {
				continue
			}
			kept = append(kept, l)
		}
		return kept
	})
}

func normalize(host string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(strings.TrimPrefix(h, "http://"), "https://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	if h == "" || strings.ContainsAny(h, " \t/\\") || !strings.Contains(h, ".") {
		return "", fmt.Errorf("host inválido: %q", host)
	}
	return h, nil
}

// mutate reescribe el archivo conservando todo lo que está fuera del bloque.
func mutate(change func([]string) []string) error {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("leer hosts: %w", err)
	}
	text := string(data)
	managed := managedLines(text)
	next := change(append([]string{}, managed...))

	block := beginMark + "\n" + strings.Join(next, "\n") + "\n" + endMark + "\n"
	var out string
	if i := strings.Index(text, beginMark); i >= 0 {
		j := strings.Index(text[i:], endMark)
		if j < 0 {
			return fmt.Errorf("bloque rkt corrupto en %s", path)
		}
		out = text[:i] + block + text[i+j+len(endMark):]
		out = strings.TrimRight(out, "\n") + "\n"
	} else {
		out = strings.TrimRight(text, "\n") + "\n\n" + block
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("escribir hosts (requiere admin): %w", err)
	}
	return nil
}

func managedLines(text string) []string {
	i := strings.Index(text, beginMark)
	if i < 0 {
		return nil
	}
	rest := text[i+len(beginMark):]
	j := strings.Index(rest, endMark)
	if j < 0 {
		return nil
	}
	out := []string{}
	for _, l := range strings.Split(rest[:j], "\n") {
		if s := strings.TrimSpace(l); s != "" && !strings.HasPrefix(s, "#") {
			out = append(out, s)
		}
	}
	return out
}
