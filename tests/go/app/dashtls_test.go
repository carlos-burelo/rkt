package app

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"rkt/internal/config"
	"rkt/internal/store"
)

// dashboard_tls=auto: el listener propio habla HTTPS con un autofirmado que
// cubre IP pública, dominio y loopback; un cliente que fija ese certificado
// (sin desactivar la verificación) conecta por cualquiera de ellos.
func TestDashboardTLSAutofirmado(t *testing.T) {
	home := t.TempDir()
	db, err := store.Open(filepath.Join(home, "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetSetting(store.KeyPublicIP, "148.236.115.31"); err != nil {
		t.Fatal(err)
	}
	p := New(filepath.Join(home, "rkt.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.db = db

	cfg := &config.Config{Dashboard: config.DashboardConfig{Host: "rkt.ejemplo.com", Listen: "127.0.0.1:0", TLS: "auto"}}
	tcfg, err := p.dashboardTLS(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tcfg.MinVersion < tls.VersionTLS12 {
		t.Fatal("TLS mínimo 1.2")
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tcfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("consola")) })}
	go srv.Serve(ln)
	defer srv.Close()

	// el cert servido es el persistido; fijarlo como raíz de confianza
	raw, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	leaf := raw.ConnectionState().PeerCertificates[0]
	raw.Close()
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	for _, sni := range []string{"148.236.115.31", "rkt.ejemplo.com", "localhost"} {
		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: pool, ServerName: sni})
		if err != nil {
			t.Fatalf("SNI %s con certificado fijado: %v", sni, err)
		}
		conn.Close()
	}
	if _, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{ServerName: "148.236.115.31"}); err == nil {
		t.Fatal("sin fijar el certificado, un autofirmado no debe verificar")
	}
	if _, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: pool, ServerName: "otro.com"}); err == nil {
		t.Fatal("el cert no debe valer para un nombre ajeno")
	}

	// HTTPS real extremo a extremo
	cl := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "rkt.ejemplo.com"}}}
	resp, err := cl.Get("https://" + ln.Addr().(*net.TCPAddr).String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if b, _ := io.ReadAll(resp.Body); string(b) != "consola" {
		t.Fatalf("cuerpo: %q", b)
	}
}
