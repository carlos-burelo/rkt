package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// NewSetupState crea un state one-time para el setup de GitHub. Devuelve
// el valor crudo (solo una vez) y guarda su hash.
func (d *DB) NewSetupState(ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := randRead(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	_, err := d.sql.Exec(
		`INSERT INTO setup_states(token_hash, expires_at) VALUES(?,?)`,
		hex.EncodeToString(sum[:]), time.Now().UTC().Add(ttl).Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("setup state: %w", err)
	}
	return token, nil
}

// ConsumeSetupState valida y borra el state. Un state solo sirve una vez.
func (d *DB) ConsumeSetupState(token string) bool {
	if token == "" {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	res, err := d.sql.Exec(
		`DELETE FROM setup_states WHERE token_hash=? AND expires_at>?`,
		hash, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// PruneSetupStates borra states caducados.
func (d *DB) PruneSetupStates() error {
	_, err := d.sql.Exec(`DELETE FROM setup_states WHERE expires_at<=?`,
		time.Now().UTC().Format(time.RFC3339))
	return err
}
