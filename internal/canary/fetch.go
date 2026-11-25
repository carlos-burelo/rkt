package canary

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Fetch hace que el servidor descargue el binario desde una URL https y lo
// guarde como canary. El sha256 es obligatorio (la fuente no está
// autenticada) y solo se conectan IPs públicas: sin SSRF hacia la red
// interna ni el endpoint de metadatos.
func (s *Store) Fetch(ctx context.Context, rawURL, label, wantSHA, by string, now time.Time) (Meta, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Meta{}, fmt.Errorf("url inválida: solo https://host/ruta")
	}
	if len(strings.TrimSpace(wantSHA)) != 64 {
		return Meta{}, fmt.Errorf("sha256 obligatorio (64 hex) para descargar de una URL")
	}
	dialer := &net.Dialer{
		Timeout: 15 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if !publicIP(host) {
				return fmt.Errorf("destino bloqueado: %s no es una IP pública", host)
			}
			return nil
		},
	}
	client := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext, ResponseHeaderTimeout: 30 * time.Second},
		Timeout:   10 * time.Minute,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Meta{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Meta{}, fmt.Errorf("descarga: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Meta{}, fmt.Errorf("descarga: http %d", resp.StatusCode)
	}
	return s.Put(resp.Body, label, wantSHA, by, now)
}

func publicIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast())
}
