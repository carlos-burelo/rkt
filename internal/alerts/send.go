package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rkt/internal/mailer"
	"rkt/internal/settings"
)

// Notification contenido de una entrega.
type Notification struct {
	FireID    int64
	ChannelID int64
	Message   string
}

// SMTPConfig arma la config SMTP desde ajustes. Hogar único: la usan el
// reparto de alertas, la API y el MCP.
func SMTPConfig(get func(string) string) mailer.Config {
	return mailer.Config{
		Host:     get(settings.KeySMTPHost),
		Port:     get(settings.KeySMTPPort),
		User:     get(settings.KeySMTPUser),
		Pass:     get(settings.KeySMTPPass),
		From:     get(settings.KeySMTPFrom),
		Security: get(settings.KeySMTPSecurity),
	}
}

// ValidChannelURL valida el destino de un canal: webhook http(s) o
// mailto:. Para correo exige SMTP configurado (fail-closed, con el comando
// que lo arregla).
func ValidChannelURL(raw string, smtp mailer.Config) error {
	raw = strings.TrimSpace(raw)
	if mailer.IsMailto(raw) {
		if _, err := mailer.ParseMailto(raw); err != nil {
			return err
		}
		if !smtp.Configured() {
			return fmt.Errorf("SMTP sin configurar: rkt set smtp_host <host> && rkt set smtp_from <correo> (y smtp_user/smtp_pass si el servidor autentica)")
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("canal inválido: usa http(s)://host/ruta o mailto:usuario@dominio")
	}
	host := strings.ToLower(u.Hostname())
	if host == "metadata.google.internal" || host == "169.254.169.254" {
		return fmt.Errorf("webhook bloqueado: no se permite el endpoint de metadatos")
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLinkLocalUnicast() {
		return fmt.Errorf("webhook bloqueado: IP link-local")
	}
	return nil
}

// Send entrega una notificación por el canal (webhook o correo). Un solo
// camino de salida: el reparto con reintentos y la prueba manual lo usan.
func Send(ctx context.Context, target string, n Notification, smtp mailer.Config, now time.Time) error {
	msg := n.Message
	if msg == "" {
		msg = fmt.Sprintf("alerta #%d", n.FireID)
	}
	if mailer.IsMailto(target) {
		to, err := mailer.ParseMailto(target)
		if err != nil {
			return err
		}
		return mailer.Send(ctx, smtp, to, "rkt · "+firstLine(msg), msg+"\n\n— rkt")
	}
	payload := map[string]any{
		"source": "rkt", "fire_id": n.FireID, "channel_id": n.ChannelID,
		"message": msg, "ts": now.Format(time.RFC3339),
	}
	switch {
	case strings.Contains(target, "hooks.slack.com"):
		payload = map[string]any{"text": "rkt · " + msg}
	case strings.Contains(target, "discord.com/api/webhooks"), strings.Contains(target, "discordapp.com/api/webhooks"):
		payload = map[string]any{"content": "rkt · " + msg}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %s", strconv.Itoa(resp.StatusCode))
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > 120 {
		return string(r[:120]) + "…"
	}
	return s
}
