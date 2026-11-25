package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rkt/internal/mailer"
)

func TestValidChannelURL(t *testing.T) {
	smtp := mailer.Config{Host: "smtp.example.com", From: "rkt@example.com"}
	cases := []struct {
		url  string
		smtp mailer.Config
		ok   bool
	}{
		{"https://hooks.slack.com/x", mailer.Config{}, true},
		{"mailto:ops@example.com", smtp, true},
		{"mailto:ops@example.com", mailer.Config{}, false}, // SMTP sin configurar
		{"mailto:no-arroba", smtp, false},
		{"ftp://x", smtp, false},
		{"http://169.254.169.254/latest", smtp, false},
		{"http://169.254.1.1/x", smtp, false},
	}
	for _, c := range cases {
		err := ValidChannelURL(c.url, c.smtp)
		if (err == nil) != c.ok {
			t.Errorf("%q: err=%v quería ok=%v", c.url, err, c.ok)
		}
	}
	err := ValidChannelURL("mailto:ops@example.com", mailer.Config{})
	if err == nil || !strings.Contains(err.Error(), "rkt set smtp_host") {
		t.Errorf("el error debe traer el comando que lo arregla: %v", err)
	}
}

func TestSendWebhook(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	err := Send(context.Background(), srv.URL, Notification{FireID: 7, ChannelID: 3, Message: "disco 95%"}, mailer.Config{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got["message"] != "disco 95%" || got["source"] != "rkt" {
		t.Fatalf("payload: %v", got)
	}
}

func TestSendWebhookErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	err := Send(context.Background(), srv.URL, Notification{FireID: 1}, mailer.Config{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("esperaba http 500: %v", err)
	}
}

func TestSendMailWithoutSMTP(t *testing.T) {
	err := Send(context.Background(), "mailto:a@example.com", Notification{FireID: 1, Message: "x"}, mailer.Config{}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("esperaba error SMTP: %v", err)
	}
}
