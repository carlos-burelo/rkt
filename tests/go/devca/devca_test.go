package devca

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCreaYReutilizaCA(t *testing.T) {
	dir := t.TempDir()
	ca, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.crt")); err != nil {
		t.Fatalf("falta ca.crt: %v", err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Thumbprint() != ca.Thumbprint() {
		t.Fatal("recargar la CA debe dar el mismo certificado")
	}
	if ca.Thumbprint() == "" || len(ca.Thumbprint()) != 40 {
		t.Fatalf("thumbprint inválido: %q", ca.Thumbprint())
	}
}

func TestGetCertificateFirmaPorHost(t *testing.T) {
	ca, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cert, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "sicesa.localhost"})
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	if _, err := cert.Leaf.Verify(x509.VerifyOptions{
		DNSName: "sicesa.localhost",
		Roots:   roots,
	}); err != nil {
		t.Fatalf("el navegador rechazaría la hoja: %v", err)
	}
	if cert.Leaf.Subject.CommonName != "sicesa.localhost" {
		t.Fatalf("CN = %q", cert.Leaf.Subject.CommonName)
	}
	// cacheado: misma hoja
	again, _ := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "sicesa.localhost"})
	if again != cert {
		t.Fatal("la hoja debe cachearse por host")
	}
	// otro host, otra hoja
	other, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "coatl-in.localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if other == cert {
		t.Fatal("hosts distintos no comparten hoja")
	}
}

func TestGetCertificateSinSNIYConIP(t *testing.T) {
	ca, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def, err := ca.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	if _, err := def.Leaf.Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Fatalf("hoja por defecto inválida: %v", err)
	}
	ip, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ip.Leaf.IPAddresses) != 1 || !ip.Leaf.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("falta SAN IP: %v", ip.Leaf.IPAddresses)
	}
}
