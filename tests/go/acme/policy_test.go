package acme

// Políticas que protegen la emisión en prod: precheck DNS (no gastar
// intentos ACME con hosts que no resuelven) y separación staging/prod
// (un cert de staging jamás debe servirse como prod).

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"rkt-proxy/internal/config"
)

func TestDNSPolicy(t *testing.T) {
	next := func(context.Context, string) error { return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := dnsPolicy(next)(ctx, "localhost"); err != nil {
		t.Fatalf("localhost resuelve: %v", err)
	}
	if err := dnsPolicy(next)(ctx, "no-existe-definitivo.invalid"); err == nil {
		t.Fatal(".invalid no debe resolver")
	}
	deny := func(context.Context, string) error { return context.Canceled }
	if err := dnsPolicy(deny)(ctx, "localhost"); err == nil {
		t.Fatal("el veto previo debe respetarse sin tocar DNS")
	}
}

func TestNewManagerStagingVsProd(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mkLive := func(staging bool) *config.Live {
		t.Helper()
		cfg := config.Default()
		cfg.ACMECacheDir = t.TempDir()
		cfg.ACMEStaging = staging
		cfg.ACMEEmail = "ops@ejemplo.com"
		if err := cfg.Finalize(); err != nil {
			t.Fatal(err)
		}
		return config.NewLive(cfg)
	}
	st := NewManager(mkLive(true), log)
	if st.Client == nil || st.Client.DirectoryURL != StagingDirectory {
		t.Fatalf("staging debe usar el directorio staging: %+v", st.Client)
	}
	stDir, ok := st.Cache.(autocert.DirCache)
	if !ok || !strings.HasSuffix(string(stDir), "staging") {
		t.Fatalf("cache staging = %q", string(stDir))
	}
	pr := NewManager(mkLive(false), log)
	if pr.Client != nil {
		t.Fatalf("prod usa el directorio por defecto, no override: %+v", pr.Client)
	}
	prDir, ok := pr.Cache.(autocert.DirCache)
	if !ok || !strings.HasSuffix(string(prDir), "prod") {
		t.Fatalf("cache prod = %q", string(prDir))
	}
	if filepath.Clean(string(stDir)) == filepath.Clean(string(prDir)) {
		t.Fatal("staging y prod comparten cache")
	}
}
