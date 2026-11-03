// Navegador de rutas para los pickers del dashboard. Solo operator+:
// leer el disco del host es información sensible, aunque sea local.
package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const fsMaxEntries = 800

type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

type fsListing struct {
	Path      string    `json:"path"`
	Parent    string    `json:"parent"`
	Entries   []fsEntry `json:"entries"`
	Truncated bool      `json:"truncated"`
}

// handleFS lista un directorio (GET /api/v1/fs?path=). Sin path devuelve
// las raíces (unidades en Windows, / en el resto).
func (d *Deps) handleFS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r, d.DB) {
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		writeJSON(w, rootsListing())
		return
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ruta inválida")
		return
	}
	listing, err := listDir(abs)
	if err != nil {
		status := http.StatusForbidden
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		writeErr(w, status, err.Error())
		return
	}
	writeJSON(w, listing)
}

// listDir lista un directorio existente. Pura respecto a HTTP: testeable.
func listDir(abs string) (fsListing, error) {
	st, err := os.Stat(abs)
	if err != nil {
		return fsListing{}, fmt.Errorf("ruta no existe: %s", abs)
	}
	if !st.IsDir() {
		abs = filepath.Dir(abs)
	}
	items, err := os.ReadDir(abs)
	if err != nil {
		return fsListing{}, fmt.Errorf("sin permiso de lectura: %s", abs)
	}

	entries := make([]fsEntry, 0, len(items))
	truncated := false
	for _, it := range items {
		if len(entries) >= fsMaxEntries {
			truncated = true
			break
		}
		name := it.Name()
		// ocultos solo en unix: en Windows los puntos son normales
		if runtime.GOOS != "windows" && strings.HasPrefix(name, ".") {
			continue
		}
		info, err := it.Info()
		if err != nil {
			continue
		}
		entries = append(entries, fsEntry{Name: name, Path: filepath.Join(abs, name), Dir: info.IsDir()})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	return fsListing{Path: abs, Parent: parent, Entries: entries, Truncated: truncated}, nil
}

// rootsListing expone los puntos de entrada del disco como una lista.
func rootsListing() fsListing {
	entries := []fsEntry{}
	if runtime.GOOS == "windows" {
		for c := 'A'; c <= 'Z'; c++ {
			p := string(c) + `:\`
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				entries = append(entries, fsEntry{Name: p, Path: p, Dir: true})
			}
		}
	} else {
		entries = append(entries, fsEntry{Name: "/", Path: "/", Dir: true})
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			entries = append(entries, fsEntry{Name: home, Path: home, Dir: true})
		}
	}
	return fsListing{Path: "", Parent: "", Entries: entries}
}
