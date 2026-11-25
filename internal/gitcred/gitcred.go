// Package gitcred: lógica de credenciales git para repos privados que no
// pasan por la GitHub App (PAT y deploy keys SSH). Pura salvo el IO de
// secretos; el orden de preferencia (App primero) lo decide quien resuelve.
//
// Secretos: <secretsDir>/git/<nombre>.pat | .key (0600). Nunca salen por
// API: solo se listan metadatos (nombre, tipo, patrón, huella).
package gitcred

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"

	"rkt/internal/store"
)

// Tipos de credencial.
const (
	KindPAT = "pat"
	KindSSH = "ssh"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,47}$`)

// ValidName nombre de credencial (también nombre de archivo: sin rutas).
func ValidName(n string) bool { return nameRe.MatchString(n) }

// ValidMatch valida el patrón "*", "owner/*" o "owner/repo".
func ValidMatch(m string) error {
	if m == "*" {
		return nil
	}
	owner, repo, ok := strings.Cut(m, "/")
	if !ok || owner == "" || repo == "" || strings.ContainsAny(m, " \\") || strings.Count(m, "/") != 1 || strings.Contains(owner, "*") {
		return fmt.Errorf("patrón inválido %q: usa *, owner/* o owner/repo", m)
	}
	return nil
}

// specificity puntaje de un patrón contra owner/repo; 0 = no aplica.
func specificity(pattern, owner, repo string) int {
	switch {
	case strings.EqualFold(pattern, owner+"/"+repo):
		return 3
	case strings.EqualFold(pattern, owner+"/*"):
		return 2
	case pattern == "*":
		return 1
	}
	return 0
}

// Pick elige la credencial más específica que aplica al repo; a igualdad,
// PAT antes que SSH (https es la URL del deploy). nil si ninguna.
func Pick(creds []store.GitCred, owner, repo string) *store.GitCred {
	var best *store.GitCred
	bestScore := 0
	for i := range creds {
		s := specificity(creds[i].Match, owner, repo) * 2
		if s == 0 {
			continue
		}
		if creds[i].Kind == KindPAT {
			s++
		}
		if s > bestScore {
			best, bestScore = &creds[i], s
		}
	}
	return best
}

// ValidPAT comprobación de forma (no de validez en GitHub: eso lo prueba
// CheckAccess). Acepta ghp_/gho_/ghu_/ghs_/github_pat_ y clásicos de 40 hex.
func ValidPAT(tok string) error {
	tok = strings.TrimSpace(tok)
	if len(tok) < 20 || len(tok) > 255 || strings.ContainsAny(tok, " \r\n\t\"'") {
		return fmt.Errorf("token con forma inválida (sin espacios, 20–255 caracteres)")
	}
	return nil
}

// secretPath ruta del archivo secreto.
func secretPath(secretsDir, name, kind string) string {
	ext := ".pat"
	if kind == KindSSH {
		ext = ".key"
	}
	return filepath.Join(secretsDir, "git", name+ext)
}

// KeyPath ruta de la llave privada de una credencial SSH.
func KeyPath(secretsDir, name string) string { return secretPath(secretsDir, name, KindSSH) }

// WriteSecret guarda el secreto (0600) de forma atómica.
func WriteSecret(secretsDir, name, kind string, data []byte) error {
	if !ValidName(name) {
		return fmt.Errorf("nombre inválido %q: minúsculas, dígitos y ._- (máx 48)", name)
	}
	p := secretPath(secretsDir, name, kind)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ReadPAT lee el token de una credencial PAT.
func ReadPAT(secretsDir, name string) (string, error) {
	b, err := os.ReadFile(secretPath(secretsDir, name, KindPAT))
	if err != nil {
		return "", fmt.Errorf("secreto de %q: %w", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// RemoveSecret borra el archivo secreto (idempotente).
func RemoveSecret(secretsDir, name, kind string) error {
	if !ValidName(name) {
		return fmt.Errorf("nombre inválido")
	}
	if err := os.Remove(secretPath(secretsDir, name, kind)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// NewDeployKey genera un par ed25519: PEM OpenSSH privado, línea pública
// authorized_keys y huella SHA256. La privada solo se guarda en el servidor.
func NewDeployKey(comment string) (privPEM []byte, pubLine, fingerprint string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", "", err
	}
	blk, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, "", "", err
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, "", "", err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))) + " " + comment
	return pem.EncodeToMemory(blk), line, ssh.FingerprintSHA256(sp), nil
}

// InspectPrivateKey valida una llave privada OpenSSH/PEM sin passphrase y
// devuelve su pública y huella (para importar una llave existente).
func InspectPrivateKey(privPEM []byte) (pubLine, fingerprint string, err error) {
	signer, err := ssh.ParsePrivateKey(privPEM)
	if err != nil {
		var pe *ssh.PassphraseMissingError
		if errors.As(err, &pe) {
			return "", "", fmt.Errorf("la llave tiene passphrase: genera una sin ella (ssh-keygen -N \"\")")
		}
		return "", "", fmt.Errorf("llave privada inválida: %w", err)
	}
	pk := signer.PublicKey()
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk))), ssh.FingerprintSHA256(pk), nil
}
