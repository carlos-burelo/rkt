package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Backup escribe una copia consistente de la DB con VACUUM INTO. Se puede
// ejecutar con el servicio vivo (SQLite toma un snapshot coherente).
func (d *DB) Backup(dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	_ = os.Remove(tmp)
	if _, err := d.sql.Exec(`VACUUM INTO ?`, tmp); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	_ = os.Chmod(dest, 0o600)
	return nil
}

// PruneBackups borra copias anteriores a cutoff.
func PruneBackups(dir string, cutoff time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			n++
		}
	}
	return n, nil
}

// CheckFile abre una DB existente y ejecuta PRAGMA integrity_check.
func CheckFile(path string) error {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return fmt.Errorf("archivo inexistente: %s", path)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var out string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&out); err != nil {
		return err
	}
	if out != "ok" {
		return fmt.Errorf("integridad: %s", out)
	}
	return nil
}
