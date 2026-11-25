// Package selfcert genera y mantiene un certificado autofirmado persistente
// para el listener propio del dashboard cuando se entra por IP o por un
// nombre sin certificado público. Hoja: solo stdlib.
//
// Se regenera si falta, está por vencer (<30 días) o la lista de nombres
// cambió (p. ej. cambió la IP pública). La huella SHA-256 se publica para
// que clientes y agentes la fijen en vez de desactivar la verificación.
package selfcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
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
	"sort"
	"strings"
	"time"
)

const (
	validity  = 825 * 24 * time.Hour // tope que aceptan los navegadores modernos
	renewSoon = 30 * 24 * time.Hour
)

// Names nombres que debe cubrir el certificado.
type Names struct {
	Hosts []string // DNS (se normalizan a minúsculas, sin vacíos)
	IPs   []string
}

func (n Names) normalized() (dns []string, ips []net.IP) {
	seen := map[string]bool{}
	for _, h := range n.Hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" && !seen[h] && net.ParseIP(h) == nil {
			seen[h] = true
			dns = append(dns, h)
		}
	}
	for _, s := range n.IPs {
		if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
			if !seen[ip.String()] {
				seen[ip.String()] = true
				ips = append(ips, ip)
			}
		}
	}
	sort.Strings(dns)
	sort.Slice(ips, func(i, j int) bool { return ips[i].String() < ips[j].String() })
	return dns, ips
}

// Fingerprint huella SHA-256 (hex en mayúsculas con :) del certificado DER.
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, 32)
	for i := 0; i < len(h); i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

// Load devuelve el certificado de dir (crea o renueva si hace falta) y su
// huella. El par vive en dir/dashboard.crt y dashboard.key (0600).
func Load(dir string, names Names, now time.Time) (*tls.Certificate, string, error) {
	crt, key := filepath.Join(dir, "dashboard.crt"), filepath.Join(dir, "dashboard.key")
	if pair, err := tls.LoadX509KeyPair(crt, key); err == nil && usable(pair, names, now) {
		return &pair, Fingerprint(pair.Certificate[0]), nil
	}
	return generate(dir, crt, key, names, now)
}

// usable el certificado cubre todos los nombres y no está por vencer.
func usable(pair tls.Certificate, names Names, now time.Time) bool {
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || now.Add(renewSoon).After(leaf.NotAfter) || now.Before(leaf.NotBefore) {
		return false
	}
	dns, ips := names.normalized()
	if len(leaf.DNSNames) != len(dns) || len(leaf.IPAddresses) != len(ips) {
		return false
	}
	have := map[string]bool{}
	for _, d := range leaf.DNSNames {
		have[d] = true
	}
	for _, ip := range leaf.IPAddresses {
		have[ip.String()] = true
	}
	for _, d := range dns {
		if !have[d] {
			return false
		}
	}
	for _, ip := range ips {
		if !have[ip.String()] {
			return false
		}
	}
	return true
}

func generate(dir, crtPath, keyPath string, names Names, now time.Time) (*tls.Certificate, string, error) {
	dns, ips := names.normalized()
	if len(dns)+len(ips) == 0 {
		return nil, "", fmt.Errorf("selfcert: sin nombres que cubrir")
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return nil, "", err
	}
	cn := "rkt dashboard"
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn, Organization: []string{"rkt"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dns,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", err
	}
	if err := writeAtomic(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		return nil, "", err
	}
	if err := writeAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})); err != nil {
		return nil, "", err
	}
	pair, err := tls.LoadX509KeyPair(crtPath, keyPath)
	if err != nil {
		return nil, "", err
	}
	return &pair, Fingerprint(der), nil
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
