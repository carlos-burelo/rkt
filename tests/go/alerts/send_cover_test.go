package alerts

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rkt/internal/mailer"
	"rkt/internal/settings"
)

func TestSMTPConfigDesdeAjustes(t *testing.T) {
	vals := map[string]string{
		settings.KeySMTPHost: "smtp.x.com", settings.KeySMTPPort: "465", settings.KeySMTPUser: "u",
		settings.KeySMTPPass: "p", settings.KeySMTPFrom: "rkt@x.com", settings.KeySMTPSecurity: "tls",
	}
	c := SMTPConfig(func(k string) string { return vals[k] })
	if c.Host != "smtp.x.com" || c.Port != "465" || c.User != "u" || c.Pass != "p" || c.From != "rkt@x.com" || c.Security != "tls" || !c.Configured() {
		t.Fatalf("cfg: %+v", c)
	}
}

// fakeSMTP acepta un correo y entrega el DATA recibido.
func fakeSMTP(t *testing.T) (string, chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan string, 1)
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
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port, got
}

func TestSendCorreoOK(t *testing.T) {
	port, got := fakeSMTP(t)
	smtp := mailer.Config{Host: "127.0.0.1", Port: port, From: "rkt@example.com", Security: mailer.None}
	long := strings.Repeat("x", 200) + "\nsegunda línea"
	err := Send(context.Background(), "mailto:ops@example.com", Notification{FireID: 3, Message: long}, smtp, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if !strings.Contains(m, "segunda") || !strings.Contains(m, "ops@example.com") {
			t.Fatalf("mensaje: %q", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sin correo")
	}
	if err := Send(context.Background(), "mailto:", Notification{FireID: 1}, smtp, time.Now()); err == nil {
		t.Fatal("mailto sin destinatarios debe fallar")
	}
}

func TestSendFormatosYFallos(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
	}))
	defer srv.Close()
	// Slack y Discord usan su propio formato (la URL contiene el host de cada uno)
	for host, key := range map[string]string{"hooks.slack.com": "text", "discord.com/api/webhooks": "content", "discordapp.com/api/webhooks": "content"} {
		got = nil
		target := srv.URL + "/?via=" + host
		if err := Send(context.Background(), target, Notification{FireID: 1, Message: "m"}, mailer.Config{}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if got[key] != "rkt · m" {
			t.Fatalf("%s: payload %v", host, got)
		}
	}
	// URL inválida (carácter de control) y servidor caído
	if err := Send(context.Background(), "http://x\x7f/", Notification{FireID: 1}, mailer.Config{}, time.Now()); err == nil {
		t.Fatal("URL inválida debe fallar")
	}
	srv.Close()
	if err := Send(context.Background(), srv.URL, Notification{FireID: 1}, mailer.Config{}, time.Now()); err == nil {
		t.Fatal("servidor caído debe fallar")
	}
}

func TestFirstLine(t *testing.T) {
	if firstLine("una\ndos") != "una" || firstLine("sola") != "sola" {
		t.Fatal("firstLine")
	}
	if got := firstLine(strings.Repeat("é", 130)); len([]rune(got)) != 121 || !strings.HasSuffix(got, "…") {
		t.Fatalf("recorte: %d", len([]rune(got)))
	}
}
