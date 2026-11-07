// Auth: usuarios locales + sesiones para api/dashboard.
// Passwords con bcrypt; sesiones bearer sha256 (nunca en claro en DB).
// Roles: admin (usuarios) > operator (mutar) > viewer (leer).
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// randRead inyectable (test de fallo de entropía).
var randRead = rand.Read

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// SessionTTL vida absoluta de sesión login (renovable con re-login).
const SessionTTL = 30 * 24 * time.Hour

// SessionIdle cierra una sesión sin actividad aunque no haya expirado.
const SessionIdle = 7 * 24 * time.Hour

// User fila pública (sin hash).
type User struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	LastLogin string `json:"last_login"`
	Disabled  bool   `json:"disabled"`
}

// Session fila pública (id = hash del bearer, nunca el bearer).
type Session struct {
	ID        string `json:"id"`
	User      string `json:"user"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
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
	rows, err := d.sql.Query(`SELECT name, role, created_at, updated_at, last_login, disabled FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.Name, &u.Role, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin, &u.Disabled); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserDump fila completa con hash, solo para backup/restore interno
// (reset). Nunca sale por API: ListUsers sigue sin hashes.
type UserDump struct {
	Name      string
	PassHash  string
	Role      string
	CreatedAt string
	UpdatedAt string
	LastLogin string
	Disabled  bool
}

// DumpUsers vuelca users con hashes para reimportar tras un reset.
func (d *DB) DumpUsers() ([]UserDump, error) {
	rows, err := d.sql.Query(`SELECT name, pass_hash, role, created_at, updated_at, last_login, disabled FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserDump{}
	for rows.Next() {
		var u UserDump
		if err := rows.Scan(&u.Name, &u.PassHash, &u.Role, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin, &u.Disabled); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ImportUsers reinserta un volcado (DB fresca tras reset). Falla si hay
// duplicados: úsalo solo sobre DB recién creada.
func (d *DB) ImportUsers(rows []UserDump) error {
	for _, u := range rows {
		if !ValidUserName(u.Name) || !validRole(u.Role) || u.PassHash == "" {
			return fmt.Errorf("usuario inválido %q", u.Name)
		}
		if _, err := d.sql.Exec(
			`INSERT INTO users(name, pass_hash, role, created_at, updated_at, last_login, disabled) VALUES(?,?,?,?,?,?,?)`,
			u.Name, u.PassHash, u.Role, u.CreatedAt, u.UpdatedAt, u.LastLogin, u.Disabled); err != nil {
			return fmt.Errorf("importar %q: %w", u.Name, err)
		}
	}
	return d.bumpVersion()
}

// DeleteUser borra; rechaza si es el último admin.
func (d *DB) DeleteUser(name string) error {
	var role string
	var admins int
	if err := d.sql.QueryRow(
		`SELECT role, (SELECT COUNT(*) FROM users WHERE role='admin') FROM users WHERE name=?`, name).
		Scan(&role, &admins); err != nil {
		return fmt.Errorf("usuario %q inexistente", name)
	}
	if role == RoleAdmin && admins <= 1 {
		return fmt.Errorf("no se puede borrar el último admin")
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
	var disabled bool
	if err := d.sql.QueryRow(`SELECT pass_hash, role, disabled FROM users WHERE name=?`, name).Scan(&hash, &role, &disabled); err != nil {
		return "", fmt.Errorf("credenciales inválidas")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", fmt.Errorf("credenciales inválidas")
	}
	if disabled {
		return "", fmt.Errorf("usuario deshabilitado")
	}
	return role, nil
}

// UpdateLastLogin registra el acceso correcto.
func (d *DB) UpdateLastLogin(name string) error {
	_, err := d.sql.Exec(`UPDATE users SET last_login=? WHERE name=?`, time.Now().UTC().Format(time.RFC3339), name)
	return err
}

// ListSessions sesiones de un usuario.
func (d *DB) ListSessions(user string) ([]Session, error) {
	rows, err := d.sql.Query(`SELECT token_hash, user, created_at, expires_at FROM sessions WHERE user=? ORDER BY created_at DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.User, &s.CreatedAt, &s.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RevokeSession borra una sesión del usuario por hash (no toca otras cuentas).
func (d *DB) RevokeSession(user, id string) error {
	res, err := d.sql.Exec(`DELETE FROM sessions WHERE user=? AND token_hash=?`, user, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("sesión inexistente")
	}
	return nil
}

// RevokeAllSessions borra todas las sesiones de un usuario.
func (d *DB) RevokeAllSessions(user string) error {
	_, err := d.sql.Exec(`DELETE FROM sessions WHERE user=?`, user)
	return err
}

// SetUserRole cambia rol (protege al último admin).
func (d *DB) SetUserRole(name, role string) error {
	if !validRole(role) {
		return fmt.Errorf("rol inválido")
	}
	var current string
	var admins int
	if err := d.sql.QueryRow(`SELECT role, (SELECT COUNT(*) FROM users WHERE role='admin') FROM users WHERE name=?`, name).
		Scan(&current, &admins); err != nil {
		return fmt.Errorf("usuario %q inexistente", name)
	}
	if current == RoleAdmin && role != RoleAdmin && admins <= 1 {
		return fmt.Errorf("no se puede degradar el último admin")
	}
	_, err := d.sql.Exec(`UPDATE users SET role=?, updated_at=? WHERE name=?`, role, time.Now().UTC().Format(time.RFC3339), name)
	return err
}

// SetUserDisabled habilita/deshabilita y revoca sesiones al deshabilitar.
func (d *DB) SetUserDisabled(name string, disabled bool) error {
	var role string
	var admins int
	if err := d.sql.QueryRow(`SELECT role, (SELECT COUNT(*) FROM users WHERE role='admin') FROM users WHERE name=?`, name).
		Scan(&role, &admins); err != nil {
		return fmt.Errorf("usuario %q inexistente", name)
	}
	if disabled && role == RoleAdmin && admins <= 1 {
		return fmt.Errorf("no se puede deshabilitar el último admin")
	}
	if _, err := d.sql.Exec(`UPDATE users SET disabled=?, updated_at=? WHERE name=?`, disabled, time.Now().UTC().Format(time.RFC3339), name); err != nil {
		return err
	}
	if disabled {
		_, _ = d.sql.Exec(`DELETE FROM sessions WHERE user=?`, name)
	}
	return nil
}

// Audit registra una acción administrativa.
type Audit struct {
	ID     int64  `json:"id"`
	Ts     string `json:"ts"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	IP     string `json:"ip"`
}

// AddAudit inserta una fila.
func (d *DB) AddAudit(actor, action, target, ip string) error {
	_, err := d.sql.Exec(`INSERT INTO audit(ts, actor, action, target, ip) VALUES(?,?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), actor, action, target, ip)
	return err
}

// ListAudit últimas N filas.
func (d *DB) ListAudit(limit int) ([]Audit, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.sql.Query(`SELECT id, ts, actor, action, target, ip FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Audit{}
	for rows.Next() {
		var a Audit
		if err := rows.Scan(&a.ID, &a.Ts, &a.Actor, &a.Action, &a.Target, &a.IP); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// NewSession crea bearer para usuario (raw solo se devuelve una vez).
func (d *DB) NewSession(user string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := randRead(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	_, err := d.sql.Exec(
		`INSERT INTO sessions(token_hash, user, created_at, expires_at, last_seen) VALUES(?,?,?,?,?)`,
		hex.EncodeToString(sum[:]), user,
		now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// SessionUser valida bearer -> (usuario, rol). Expiradas o inactivas se
// purgan; la actividad renueva el reloj de idle sin tocar el TTL absoluto.
func (d *DB) SessionUser(token string) (string, string, bool) {
	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])
	var user, exp, created, seen string
	if err := d.sql.QueryRow(
		`SELECT user, expires_at, created_at, last_seen FROM sessions WHERE token_hash=?`, want).
		Scan(&user, &exp, &created, &seen); err != nil {
		return "", "", false
	}
	now := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, exp); err != nil || now.After(t) {
		_, _ = d.sql.Exec(`DELETE FROM sessions WHERE token_hash=?`, want)
		return "", "", false
	}
	if seen == "" {
		seen = created
	}
	last, err := time.Parse(time.RFC3339, seen)
	if err != nil || now.Sub(last) > SessionIdle {
		_, _ = d.sql.Exec(`DELETE FROM sessions WHERE token_hash=?`, want)
		return "", "", false
	}
	var role string
	var disabled bool
	if err := d.sql.QueryRow(`SELECT role, disabled FROM users WHERE name=?`, user).Scan(&role, &disabled); err != nil || disabled {
		return "", "", false
	}
	_, _ = d.sql.Exec(`UPDATE sessions SET last_seen=? WHERE token_hash=?`, now.Format(time.RFC3339), want)
	return user, role, true
}

// PurgeSessions borra expiradas y sin actividad. Devuelve cuántas quitó.
func (d *DB) PurgeSessions() (int64, error) {
	now := time.Now().UTC()
	res, err := d.sql.Exec(
		`DELETE FROM sessions WHERE expires_at<=? OR (CASE WHEN last_seen='' THEN created_at ELSE last_seen END)<=?`,
		now.Format(time.RFC3339), now.Add(-SessionIdle).Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TokenHash id público de una sesión (hash sha256 del bearer).
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// DeleteSession logout (por bearer raw).
func (d *DB) DeleteSession(token string) {
	_, _ = d.sql.Exec(`DELETE FROM sessions WHERE token_hash=?`, TokenHash(token))
}
