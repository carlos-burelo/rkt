package pg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedHBA(t *testing.T) {
	b := ManagedHBA("rkt", []string{"web", "api", "web"})
	if !strings.HasPrefix(b, hbaBegin+"\n") || !strings.Contains(b, hbaEnd+"\n") {
		t.Fatalf("delimitadores:\n%s", b)
	}
	if got := strings.Count(b, "127.0.0.1/32"); got != 2 {
		t.Fatalf("una línea por rol (dedup):\n%s", b)
	}
	for _, want := range []string{"host    rkt    web", "host    rkt    api", "trust"} {
		if !strings.Contains(b, want) {
			t.Fatalf("falta %q:\n%s", want, b)
		}
	}
	if got := ManagedHBA("rkt", nil); got != "" {
		t.Fatalf("sin roles no debe haber bloque: %q", got)
	}
}

func TestApplyHBAFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pg_hba.conf")
	orig := "# comment\nlocal all all trust\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := ApplyHBAFile(path, ManagedHBA("rkt", []string{"web"}))
	if err != nil || !changed {
		t.Fatalf("primera aplicación: %v %v", changed, err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(raw), hbaBegin) || !strings.Contains(string(raw), "# comment") {
		t.Fatalf("bloque antepuesto y resto intacto:\n%s", raw)
	}
	if _, err := os.Stat(path + ".rkt-bak"); err != nil {
		t.Fatalf("backup exigido: %v", err)
	}
	// Idempotente.
	changed, err = ApplyHBAFile(path, ManagedHBA("rkt", []string{"web"}))
	if err != nil || changed {
		t.Fatalf("repetir no debe cambiar: %v %v", changed, err)
	}
	// Reemplazo (no duplicado).
	changed, err = ApplyHBAFile(path, ManagedHBA("rkt", []string{"api"}))
	if err != nil || !changed {
		t.Fatalf("cambio de roles: %v %v", changed, err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Count(string(raw), hbaBegin) != 1 {
		t.Fatalf("un solo bloque:\n%s", raw)
	}
}

func TestSuperDSN(t *testing.T) {
	if got := SuperDSN("127.0.0.1", 5432, "postgres", "", "rkt"); got != "postgres://postgres@127.0.0.1:5432/rkt?sslmode=disable&connect_timeout=10" {
		t.Fatalf("got %q", got)
	}
	if got := SuperDSN("127.0.0.1", 5432, "postgres", "root", "rkt"); !strings.Contains(got, "postgres:root@") {
		t.Fatalf("con password: %q", got)
	}
	if got := SuperDSN("::1", 5433, "postgres", "", "rkt"); !strings.Contains(got, "[::1]:5433") {
		t.Fatalf("ipv6: %q", got)
	}
}

func TestBackupToErrores(t *testing.T) {
	if err := BackupTo(context.Background(), "", "127.0.0.1", 5432, "u", "", "rkt", "web", "x"); err == nil {
		t.Fatal("sin binario debe fallar")
	}
	if err := BackupTo(context.Background(), "pg_dump", "127.0.0.1", 5432, "u", "", "rkt", "MAL", "x"); err == nil {
		t.Fatal("esquema inválido debe fallar")
	}
}
