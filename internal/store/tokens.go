package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// APIToken token de automatización (CI, scripts). Nunca se devuelve el
// valor crudo salvo en el alta; la DB guarda sha256.
type APIToken struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	LastUsed  string `json:"last_used"`
}

// CreateToken crea un token con rol y TTL. Devuelve el valor crudo una vez.
func (d *DB) CreateToken(name, role string, ttl time.Duration) (string, error) {
	if name == "" || len(name) > 80 {
		return "", fmt.Errorf("nombre de token inválido")
	}
	if !validRole(role) {
		return "", fmt.Errorf("rol inválido (admin|operator|viewer)")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("ttl inválido")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := "rkt_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	_, err := d.sql.Exec(
		`INSERT INTO api_tokens(name, token_hash, role, created_at, expires_at) VALUES(?,?,?,?,?)`,
		name, hex.EncodeToString(sum[:]), role,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	return token, nil
}

// ListTokens sin hashes.
func (d *DB) ListTokens() ([]APIToken, error) {
	rows, err := d.sql.Query(
		`SELECT id, name, role, created_at, expires_at, last_used FROM api_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.Name, &t.Role, &t.CreatedAt, &t.ExpiresAt, &t.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken borra por id.
func (d *DB) RevokeToken(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM api_tokens WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token inexistente")
	}
	return nil
}

// TokenUser valida un token de API y devuelve su rol.
func (d *DB) TokenUser(token string) (string, string, bool) {
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	var name, role, exp string
	if err := d.sql.QueryRow(
		`SELECT name, role, expires_at FROM api_tokens WHERE token_hash=?`, hash).
		Scan(&name, &role, &exp); err != nil {
		return "", "", false
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil || time.Now().UTC().After(t) {
		_, _ = d.sql.Exec(`DELETE FROM api_tokens WHERE token_hash=?`, hash)
		return "", "", false
	}
	_, _ = d.sql.Exec(`UPDATE api_tokens SET last_used=? WHERE token_hash=?`,
		time.Now().UTC().Format(time.RFC3339), hash)
	return name, role, true
}
