package pg

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupInfo volcado en disco (para la pestaña Copias real).
type BackupInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

// validBackupName solo pg-<app>-<ts>.dump (sin rutas ni ..).
func validBackupName(app, name string) error {
	if strings.ContainsAny(name, `/\`+string(filepath.Separator)) && filepath.Base(name) != name {
		return fmt.Errorf("nombre de backup inválido")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("nombre de backup inválido")
	}
	if app != "" {
		if !strings.HasPrefix(name, "pg-"+app+"-") || !strings.HasSuffix(name, ".dump") {
			return fmt.Errorf("el backup %q no es de la app %q", name, app)
		}
	} else if !strings.HasSuffix(name, ".dump") {
		return fmt.Errorf("nombre de backup inválido")
	}
	if len(name) > 128 {
		return fmt.Errorf("nombre de backup demasiado largo")
	}
	return nil
}

// ListBackups lista pg-*.dump del dir (orden reciente primero).
func ListBackups(dir, app string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}
	out := []BackupInfo{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasPrefix(n, "pg-") || !strings.HasSuffix(n, ".dump") {
			continue
		}
		if app != "" && !strings.HasPrefix(n, "pg-"+app+"-") {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Name: n, Path: filepath.Join(dir, n),
			Size: st.Size(), Mtime: st.ModTime().UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mtime > out[j].Mtime })
	if out == nil {
		out = []BackupInfo{}
	}
	return out, nil
}

// BackupPath resuelve un nombre de backup dentro del dir (sin traversal).
func BackupPath(dir, app, name string) (string, error) {
	base := filepath.Base(strings.TrimSpace(name))
	if err := validBackupName(app, base); err != nil {
		return "", err
	}
	p := filepath.Join(dir, base)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("backup no encontrado: %s", base)
	}
	return p, nil
}

// DeleteBackup borra un dump del dir (nombre base, sin rutas).
func DeleteBackup(dir, app, name string) error {
	p, err := BackupPath(dir, app, name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}
