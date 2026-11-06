package app

import (
	"crypto/tls"
	"fmt"
	"strings"

	"rkt-proxy/internal/config"
)

// customCerts certificados propios cargados al arrancar. Exacto gana a
// wildcard; si ninguno coincide se delega a ACME/CA local.
type customCerts struct {
	exact map[string]*tls.Certificate
	wild  []wildCert
}

type wildCert struct {
	suffix string
	cert   *tls.Certificate
}

func loadCustomCerts(list []config.TLSCert) (*customCerts, error) {
	c := &customCerts{exact: map[string]*tls.Certificate{}}
	for _, item := range list {
		cert, err := tls.LoadX509KeyPair(item.CertFile, item.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("tls_certs %v: %w", item.Hosts, err)
		}
		for _, raw := range item.Hosts {
			h := strings.ToLower(strings.TrimSpace(raw))
			if strings.HasPrefix(h, "*.") {
				c.wild = append(c.wild, wildCert{suffix: h[1:], cert: &cert})
				continue
			}
			c.exact[h] = &cert
		}
	}
	return c, nil
}

func (c *customCerts) get(server string) *tls.Certificate {
	if c == nil {
		return nil
	}
	server = strings.ToLower(strings.TrimSpace(server))
	if i := strings.LastIndex(server, ":"); i >= 0 {
		server = server[:i]
	}
	if cert := c.exact[server]; cert != nil {
		return cert
	}
	for _, w := range c.wild {
		if strings.HasSuffix(server, w.suffix) && len(server) > len(w.suffix) {
			return w.cert
		}
	}
	return nil
}

func wrapCert(custom *customCerts, base func(*tls.ClientHelloInfo) (*tls.Certificate, error)) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if cert := custom.get(hello.ServerName); cert != nil {
			return cert, nil
		}
		return base(hello)
	}
}
