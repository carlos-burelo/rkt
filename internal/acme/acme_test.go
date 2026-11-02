package acme

import (
	"context"
	"os"
	"testing"

	"rkt-proxy/internal/config"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-acme-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestHostPolicy(t *testing.T) {
	path := writeTemp(t, `
routes:
  - hosts: ["app.ejemplo.com"]
    target: "http://127.0.0.1:3000"
  - hosts: ["*.ejemplo.com"]
    target: "http://127.0.0.1:4000"
`)
	defer os.Remove(path)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	policy := HostPolicy(config.NewLive(cfg))
	ctx := context.Background()

	for _, h := range []string{"app.ejemplo.com", "otro.ejemplo.com", "APP.EJEMPLO.COM"} {
		if err := policy(ctx, h); err != nil {
			t.Fatalf("host %q debe pasar: %v", h, err)
		}
	}
	for _, h := range []string{"ejemplo.com", "evil.com", ""} {
		if err := policy(ctx, h); err == nil {
			t.Fatalf("host %q debe bloquearse", h)
		}
	}
}
