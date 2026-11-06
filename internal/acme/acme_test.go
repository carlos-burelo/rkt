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

func TestLimitPolicy(t *testing.T) {
	next := func(context.Context, string) error { return nil }
	p := limitPolicy(1, next)
	ctx := context.Background()
	if err := p(ctx, "a.test"); err != nil {
		t.Fatalf("primera emisión: %v", err)
	}
	// El mismo host ya visto no consume cupo (reintento de emisión).
	if err := p(ctx, "a.test"); err != nil {
		t.Fatalf("host ya visto: %v", err)
	}
	if err := p(ctx, "b.test"); err == nil {
		t.Fatal("segunda emisión en la ventana debe fallar")
	}
	if err := limitPolicy(-1, next)(ctx, "c.test"); err != nil {
		t.Fatalf("límite desactivado: %v", err)
	}
}

func TestHostPolicy(t *testing.T) {
	cfg := config.Default()
	cfg.Routes = []config.Route{
		{Hosts: []string{"app.ejemplo.com"}, Target: "http://127.0.0.1:3000"},
		{Hosts: []string{"*.ejemplo.com"}, Target: "http://127.0.0.1:4000"},
	}
	if err := cfg.Finalize(); err != nil {
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
