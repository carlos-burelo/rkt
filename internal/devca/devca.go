// Package devca: CA propia para HTTPS en desarrollo. Firma un certificado
// por host al vuelo (ECDSA P-256) para que dev use los mismos puertos que
// producción (80/443) sin ACME, que no puede emitir para .localhost/.test.
package devca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 397 * 24 * time.Hour // tope de Chrome/Safari para certs emitidos
)

// CA es la autoridad local: cert+clave en disco, hojas cacheadas en memoria.
type CA struct {
	cert     *x509.Certificate
	key      *ecdsa.PrivateKey
	certPath string
	thumb    string // SHA-1 hex (formato de thumbprint estándar)

	mu     sync.Mutex
	leaves map[string]*tls.Certificate
}

// Load abre la CA del directorio o la crea si no existe (ca.crt/ca.key).
func Load(dir string) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("crear dir CA: %w", err)
	}
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")

	if _, err := os.Stat(certPath); err == nil {
		return load(certPath, keyPath)
	}
	return create(certPath, keyPath)
}

func load(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("leer CA: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("leer clave CA: %w", err)
	}
	cb, _ := pem.Decode(certPEM)
	if cb == nil {
		return nil, fmt.Errorf("CA sin PEM: %s", certPath)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsear CA: %w", err)
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return nil, fmt.Errorf("clave CA sin PEM: %s", keyPath)
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsear clave CA: %w", err)
	}
	return &CA{cert: cert, key: key, certPath: certPath, thumb: thumbprint(cert.Raw), leaves: map[string]*tls.Certificate{}}, nil
}

func create(certPath, keyPath string) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "rkt dev CA", Organization: []string{"rkt dev"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("crear CA: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key, certPath: certPath, thumb: thumbprint(der), leaves: map[string]*tls.Certificate{}}, nil
}

// CertPath ruta del PEM público (lo que se instala en el almacén).
func (c *CA) CertPath() string { return c.certPath }

// Thumbprint SHA-1 hex en mayúsculas del certificado de la CA.
func (c *CA) Thumbprint() string { return c.thumb }

// GetCertificate firma (y cachea) una hoja para el ServerName del handshake.
// Sin SNI usa localhost.
func (c *CA) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hello.ServerName), "."))
	if name == "" {
		name = "localhost"
	}
	c.mu.Lock()
	if cert, ok := c.leaves[name]; ok {
		c.mu.Unlock()
		return cert, nil
	}
	c.mu.Unlock()

	cert, err := c.issue(name)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.leaves[name] = cert
	c.mu.Unlock()
	return cert, nil
}

func (c *CA) issue(name string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:   serial,
		Subject:        pkix.Name{CommonName: name, Organization: []string{"rkt dev"}},
		NotBefore:      now.Add(-time.Hour),
		NotAfter:       now.Add(leafValidity),
		KeyUsage:       x509.KeyUsageDigitalSignature,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		AuthorityKeyId: c.cert.SubjectKeyId,
	}
	if ip := net.ParseIP(name); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{name}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return nil, fmt.Errorf("firmar %s: %w", name, err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der, c.cert.Raw}, PrivateKey: key, Leaf: leaf}, nil
}

func thumbprint(der []byte) string {
	sum := sha1.Sum(der) //nolint:gosec // SHA-1 solo para thumbprint, no es seguridad
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
