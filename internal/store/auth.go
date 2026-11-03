// Auth: usuarios locales + sesiones para api/dashboard.
// Passwords con bcrypt; sesiones bearer sha256 (nunca en claro en DB).
// Roles: admin (usuarios) > operator (mutar) > viewer (leer).
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// SessionTTL vida de sesión login (renovable con re-login).
const SessionTTL = 30 * 24 * time.Hour

// User fila pública (sin hash).
type User struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

func validRole(r string) bool {
	return r == RoleAdmin || r == RoleOperator || r == RoleViewer
}

// ValidUserName [a-z0-9-_] hasta 64. Canónico (CLI y API lo usan).
func ValidUserName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

// CountUsers para bootstrap (primer admin por CLI).
func (d *DB) CountUsers() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// ValidateCredential valida sin tocar DB (para errores 400 exactos).
func ValidateCredential(name, password string) error {
	if !ValidUserName(name) {
		return fmt.Errorf("usuario inválido [a-z0-9-_], máx 64")
	}
	if len(password) < 12 {
		return fmt.Errorf("password mínimo 12 caracteres")
	}
	return nil
}

// CreateUser alta con password hasheado. Primera cuenta la crea el CLI.
func (d *DB) CreateUser(name, password, role string) error {
	if err := ValidateCredential(name, password); err != nil {
		return err
	}
	if !validRole(role) {
		return fmt.Errorf("rol inválido (admin|operator|viewer)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(
		`INSERT INTO users(name, pass_hash, role, created_at) VALUES(?,?,?,?)`,
		name, string(hash), role, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("alta %q: %w", name, err)
	}
	return d.bumpVersion()
}

// ListUsers sin hashes.
func (d *DB) ListUsers() ([]User, error) {
	rows, err := d.sql.Query(`SELECT name, role, created_at FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Name, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUser borra; rechaza si es el último admin.
func (d *DB) DeleteUser(name string) error {
	var role string
	if err := d.sql.QueryRow(`SELECT role FROM users WHERE name=?`, name).Scan(&role); err != nil {
		return fmt.Errorf("usuario %q inexistente", name)
	}
	if role == RoleAdmin {
		var n int
		if err := d.sql.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return fmt.Errorf("no se puede borrar el último admin")
		}
	}
	if _, err := d.sql.Exec(`DELETE FROM users WHERE name=?`, name); err != nil {
		return err
	}
	_, _ = d.sql.Exec(`DELETE FROM sessions WHERE user=?`, name)
	return d.bumpVersion()
}

// SetPassword rota password e invalida sesiones (re-login).
func (d *DB) SetPassword(name, password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password mínimo 12 caracteres")
	}
	var exists string
	if err := d.sql.QueryRow(`SELECT name FROM users WHERE name=?`, name).Scan(&exists); err != nil {
		return fmt.Errorf("usuario %q inexistente", name)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := d.sql.Exec(`UPDATE users SET pass_hash=? WHERE name=?`, string(hash), name); err != nil {
		return err
	}
	_, _ = d.sql.Exec(`DELETE FROM sessions WHERE user=?`, name)
	return d.bumpVersion()
}

// CheckPassword compara password (tiempo constante en hash bcrypt).
func (d *DB) CheckPassword(name, password string) (string, error) {
	var hash, role string
	if err := d.sql.QueryRow(`SELECT pass_hash, role FROM users WHERE name=?`, name).Scan(&hash, &role); err != nil {
		return "", fmt.Errorf("credenciales inválidas")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", fmt.Errorf("credenciales inválidas")
	}
	return role, nil
}

// NewSession crea bearer para usuario (raw solo se devuelve una vez).
func (d *DB) NewSession(user string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	_, err := d.sql.Exec(
		`INSERT INTO sessions(token_hash, user, created_at, expires_at) VALUES(?,?,?,?)`,
		hex.EncodeToString(sum[:]), user,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// SessionUser valida bearer -> (usuario, rol). Expiradas se purgan.
func (d *DB) SessionUser(token string) (string, string, bool) {
	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])
	var hash, user, exp string
	if err := d.sql.QueryRow(
		`SELECT token_hash, user, expires_at FROM sessions WHERE token_hash=?`, want).
		Scan(&hash, &user, &exp); err != nil {
		return "", "", false
	}
	if subtle.ConstantTimeCompare([]byte(hash), []byte(want)) != 1 {
		return "", "", false
	}
	if t, err := time.Parse(time.RFC3339, exp); err != nil || time.Now().UTC().After(t) {
		_, _ = d.sql.Exec(`DELETE FROM sessions WHERE token_hash=?`, want)
		return "", "", false
	}
	var role string
	if err := d.sql.QueryRow(`SELECT role FROM users WHERE name=?`, user).Scan(&role); err != nil {
		return "", "", false
	}
	return user, role, true
}

// DeleteSession logout (por bearer raw).
func (d *DB) DeleteSession(token string) {
	sum := sha256.Sum256([]byte(token))
	_, _ = d.sql.Exec(`DELETE FROM sessions WHERE token_hash=?`, hex.EncodeToString(sum[:]))
}
