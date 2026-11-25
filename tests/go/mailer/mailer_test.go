package mailer

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTP servidor mínimo en loopback; entrega lo recibido en DATA.
func fakeSMTP(t *testing.T) (port string, got chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		w("220 fake")
		var data strings.Builder
		in := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if in {
				if line == ".\r\n" {
					in = false
					got <- data.String()
					w("250 ok")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(line); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				w("250 fake")
			case strings.HasPrefix(cmd, "DATA"):
				in = true
				w("354 go")
			case strings.HasPrefix(cmd, "QUIT"):
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, got
}

func TestSend(t *testing.T) {
	port, got := fakeSMTP(t)
	cfg := Config{Host: "127.0.0.1", Port: port, From: "rkt@example.com", Security: None}
	err := Send(context.Background(), cfg, []string{"ops@example.com"}, "alerta\r\nBcc: x@evil.com", "linea1\nlinea2")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if strings.Contains(m, "\r\nBcc:") {
			t.Fatalf("inyección de cabecera: %q", m)
		}
		if !strings.Contains(m, "To: ops@example.com") || !strings.Contains(m, "linea2") {
			t.Fatalf("mensaje: %q", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sin DATA")
	}
}

func TestSendErrors(t *testing.T) {
	if err := Send(context.Background(), Config{}, []string{"a@b.co"}, "s", "b"); err == nil {
		t.Fatal("sin config debe fallar")
	}
	// STARTTLS exigido: el fake no lo ofrece, no se degrada a claro.
	port, _ := fakeSMTP(t)
	cfg := Config{Host: "127.0.0.1", Port: port, From: "rkt@example.com"}
	if err := Send(context.Background(), cfg, []string{"a@b.co"}, "s", "b"); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("esperaba error STARTTLS: %v", err)
	}
}

func TestParseMailto(t *testing.T) {
	to, err := ParseMailto("mailto:a@x.com, b@y.org")
	if err != nil || len(to) != 2 || to[1] != "b@y.org" {
		t.Fatalf("%v %v", to, err)
	}
	for _, bad := range []string{"https://x.com", "mailto:", "mailto:no-arroba", "mailto:A <a@x.com>"} {
		if _, err := ParseMailto(bad); err == nil {
			t.Errorf("%q debía fallar", bad)
		}
	}
	if !IsMailto(" MAILTO:a@x.com") || IsMailto("https://x") {
		t.Error("IsMailto")
	}
}
