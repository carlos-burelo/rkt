package selfcert

import (
	"crypto/x509"
	"strings"
	"testing"
	"time"
)

func leaf(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadGeneraYReusa(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	names := Names{Hosts: []string{"Panel.Ejemplo.com", "localhost", ""}, IPs: []string{"148.236.115.31", "127.0.0.1", "no-ip"}}
	c1, fp1, err := Load(dir, names, now)
	if err != nil {
		t.Fatal(err)
	}
	l := leaf(t, c1.Certificate[0])
	if len(l.DNSNames) != 2 || l.DNSNames[1] != "panel.ejemplo.com" || len(l.IPAddresses) != 2 {
		t.Fatalf("SAN: dns=%v ips=%v", l.DNSNames, l.IPAddresses)
	}
	if err := l.VerifyHostname("148.236.115.31"); err != nil {
		t.Fatalf("debe cubrir la IP pública: %v", err)
	}
	if !strings.Contains(fp1, ":") || len(fp1) != 95 {
		t.Fatalf("huella: %q", fp1)
	}
	// mismo set de nombres (en otro orden) reusa el archivo: misma huella
	_, fp2, err := Load(dir, Names{Hosts: []string{"localhost", "panel.ejemplo.com"}, IPs: []string{"127.0.0.1", "148.236.115.31"}}, now.Add(time.Hour))
	if err != nil || fp2 != fp1 {
		t.Fatalf("debía reusar: %q vs %q (%v)", fp2, fp1, err)
	}
}

func TestLoadRegeneraSiCambianNombresOVence(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	_, fp1, err := Load(dir, Names{IPs: []string{"1.2.3.4"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	// cambió la IP pública
	_, fp2, err := Load(dir, Names{IPs: []string{"5.6.7.8"}}, now)
	if err != nil || fp2 == fp1 {
		t.Fatalf("cambio de IP debe regenerar: %v", err)
	}
	// a <30 días de vencer
	_, fp3, err := Load(dir, Names{IPs: []string{"5.6.7.8"}}, now.Add(800*24*time.Hour))
	if err != nil || fp3 == fp2 {
		t.Fatalf("por vencer debe renovar: %v", err)
	}
}

func TestLoadSinNombres(t *testing.T) {
	if _, _, err := Load(t.TempDir(), Names{Hosts: []string{" "}, IPs: []string{"x"}}, time.Now()); err == nil {
		t.Fatal("sin nombres válidos debe fallar")
	}
}
