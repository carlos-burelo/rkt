package store

import (
	"fmt"
	"time"
)

// GitCred metadatos de una credencial git guardada. El secreto (PAT o
// llave privada) vive en el directorio de secretos con permisos de
// máquina, nunca en la DB ni en respuestas de API.
type GitCred struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`  // pat | ssh
	Match       string `json:"match"` // "*", "owner/*" o "owner/repo"
	PublicKey   string `json:"public_key,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// AddGitCred registra una credencial (nombre único).
func (d *DB) AddGitCred(c GitCred) error {
	_, err := d.sql.Exec(`INSERT INTO git_credentials(name, kind, match, public_key, fingerprint, created_at) VALUES(?,?,?,?,?,?)`,
		c.Name, c.Kind, c.Match, c.PublicKey, c.Fingerprint, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("credencial %q: %w", c.Name, err)
	}
	return d.bumpVersion()
}

// ListGitCreds lista metadatos por nombre.
func (d *DB) ListGitCreds() ([]GitCred, error) {
	rows, err := d.sql.Query(`SELECT name, kind, match, public_key, fingerprint, created_at FROM git_credentials ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GitCred{}
	for rows.Next() {
		var c GitCred
		if err := rows.Scan(&c.Name, &c.Kind, &c.Match, &c.PublicKey, &c.Fingerprint, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteGitCred borra una credencial; error si no existe.
func (d *DB) DeleteGitCred(name string) error {
	res, err := d.sql.Exec(`DELETE FROM git_credentials WHERE name=?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("credencial %q inexistente", name)
	}
	return d.bumpVersion()
}
