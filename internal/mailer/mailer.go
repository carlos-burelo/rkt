// Package mailer envía correo por SMTP (STARTTLS, TLS implícito o claro).
// Hoja: solo stdlib. La config llega ya resuelta; aquí no se lee la DB.
package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// Seguridad del transporte.
const (
	STARTTLS = "starttls" // 587 (default): exige la extensión, sin degradar a claro
	TLS      = "tls"      // 465: TLS implícito
	None     = "none"     // 25/dev: sin cifrar
)

// Config conexión SMTP. Auth solo si User != "".
type Config struct {
	Host     string
	Port     string
	User     string
	Pass     string
	From     string
	Security string
}

// Configured true si hay lo mínimo para enviar.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.From) != ""
}

func (c Config) security() string {
	switch strings.ToLower(strings.TrimSpace(c.Security)) {
	case TLS:
		return TLS
	case None:
		return None
	}
	return STARTTLS
}

func (c Config) port() string {
	if p := strings.TrimSpace(c.Port); p != "" {
		return p
	}
	if c.security() == TLS {
		return "465"
	}
	return "587"
}

// ParseMailto extrae destinatarios de "mailto:a@x.com,b@y.com".
func ParseMailto(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if !IsMailto(raw) {
		return nil, fmt.Errorf("correo inválido: usa mailto:usuario@dominio")
	}
	var out []string
	for _, p := range strings.Split(raw[len("mailto:"):], ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		a, err := mail.ParseAddress(p)
		if err != nil || a.Name != "" || a.Address != p {
			return nil, fmt.Errorf("destinatario inválido: %q", p)
		}
		out = append(out, a.Address)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("correo sin destinatarios")
	}
	return out, nil
}

// IsMailto true si la URL de un canal es de correo.
func IsMailto(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "mailto:")
}

// Send entrega un texto plano a todos los destinatarios.
func Send(ctx context.Context, c Config, to []string, subject, body string) error {
	if !c.Configured() {
		return fmt.Errorf("SMTP sin configurar: rkt set smtp_host <host> && rkt set smtp_from <correo>")
	}
	from, err := mail.ParseAddress(c.From)
	if err != nil {
		return fmt.Errorf("smtp_from inválido: %w", err)
	}
	host := strings.TrimSpace(c.Host)
	addr := net.JoinHostPort(host, c.port())
	d := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if c.security() == TLS {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp conectar %s: %w", addr, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	cl, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp saludo: %w", err)
	}
	defer cl.Close()
	if c.security() == STARTTLS {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return fmt.Errorf("el servidor SMTP no ofrece STARTTLS (usa smtp_security=tls o none)")
		}
		if err := cl.StartTLS(&tls.Config{ServerName: host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if c.User != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.User, c.Pass, host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := cl.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	for _, r := range to {
		if err := cl.Rcpt(r); err != nil {
			return fmt.Errorf("smtp destinatario %s: %w", r, err)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(message(from.String(), to, subject, body))); err != nil {
		return fmt.Errorf("smtp cuerpo: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp cierre: %w", err)
	}
	return cl.Quit()
}

// message arma el mensaje. El asunto se sanea (sin CR/LF: inyección de cabeceras).
func message(from string, to []string, subject, body string) string {
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	b.WriteString("\r\n")
	return b.String()
}
