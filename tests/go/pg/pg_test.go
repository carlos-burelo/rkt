package pg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidIdent(t *testing.T) {
	for _, ok := range []string{"rkt", "app_1", "_x", "a$b", "a1"} {
		if err := ValidIdent(ok); err != nil {
			t.Fatalf("%q válido: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Mayus", "con espacio", "1abc", "con-guion", strings.Repeat("a", 64)} {
		if err := ValidIdent(bad); err == nil {
			t.Fatalf("%q debió fallar", bad)
		}
	}
	if err := ValidIdent(strings.Repeat("a", 63)); err != nil {
		t.Fatalf("63 ok: %v", err)
	}
}

func TestQuoteIdent(t *testing.T) {
	if got := QuoteIdent("rkt"); got != `"rkt"` {
		t.Fatalf("got %q", got)
	}
	if got := QuoteIdent(`a"b`); got != `"a""b"` {
		t.Fatalf("escape: %q", got)
	}
}

func TestSchemaForApp(t *testing.T) {
	got, err := SchemaForApp("Mi-App")
	if err != nil || got != "mi_app" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := SchemaForApp(""); err == nil {
		t.Fatal("vacía debe fallar")
	}
	if _, err := SchemaForApp("hola mundo"); err == nil {
		t.Fatal("espacio debe fallar")
	}
	if _, err := SchemaForApp(strings.Repeat("a", 64)); err == nil {
		t.Fatal("larga debe fallar")
	}
}

func TestConnectionURL(t *testing.T) {
	if got := ConnectionURL("127.0.0.1", 5432, "rkt", "web"); got != "postgres://web@127.0.0.1:5432/rkt?sslmode=disable" {
		t.Fatalf("got %q", got)
	}
	if got := ConnectionURL("::1", 5433, "rkt", "web"); !strings.Contains(got, "[::1]:5433") {
		t.Fatalf("ipv6 con corchetes: %q", got)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, h := range []string{"localhost", "127.0.0.1", "::1", "[::1]", " LOCALHOST "} {
		if !IsLoopbackHost(h) {
			t.Fatalf("%q es loopback", h)
		}
	}
	for _, h := range []string{"", "example.com", "192.168.1.1", "10.0.0.1"} {
		if IsLoopbackHost(h) {
			t.Fatalf("%q no es loopback", h)
		}
	}
}

func TestPlanProvisionDedicated(t *testing.T) {
	p, err := PlanProvision("Mi-App", "", "", "127.0.0.1", 5432)
	if err != nil {
		t.Fatal(err)
	}
	if p.Schema != "mi_app" || p.Role != "mi_app" || p.Mode != "dedicated" || p.DB != "rkt" {
		t.Fatalf("plan: %+v", p)
	}
	if p.URL != "postgres://mi_app@127.0.0.1:5432/rkt?sslmode=disable" {
		t.Fatalf("url: %q", p.URL)
	}
	joined := strings.Join(p.Statements, "\n")
	for _, want := range []string{
		`CREATE SCHEMA IF NOT EXISTS "mi_app"`,
		`CREATE ROLE "mi_app" WITH LOGIN`,
		`GRANT USAGE, CREATE ON SCHEMA "mi_app" TO "mi_app"`,
		`ALTER DEFAULT PRIVILEGES FOR ROLE "mi_app" IN SCHEMA "mi_app"`,
		`ALTER ROLE "mi_app" IN DATABASE "rkt" SET search_path = "mi_app"`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %q en:\n%s", want, joined)
		}
	}
}

func TestPlanProvisionShared(t *testing.T) {
	p, err := PlanProvision("web", "shared", "analytics", "127.0.0.1", 5433)
	if err != nil {
		t.Fatal(err)
	}
	if p.Role != SharedRole || p.DB != "analytics" {
		t.Fatalf("plan: %+v", p)
	}
	joined := strings.Join(p.Statements, "\n")
	if strings.Contains(joined, "CREATE ROLE") {
		t.Fatalf("shared no debe crear rol:\n%s", joined)
	}
	if !strings.Contains(joined, `TO "rkt_apps"`) {
		t.Fatalf("shared debe dar a rkt_apps:\n%s", joined)
	}
	if got := EnsureSharedRole(); !strings.Contains(got, `"rkt_apps"`) {
		t.Fatalf("ensure: %q", got)
	}
}

func TestPlanProvisionInvalido(t *testing.T) {
	for _, tc := range [][3]string{
		{"", "dedicated", "rkt"},
		{"hola mundo", "dedicated", "rkt"},
		{"web", "otro", "rkt"},
		{"web", "dedicated", "MiDB"},
	} {
		if _, err := PlanProvision(tc[0], tc[1], tc[2], "127.0.0.1", 5432); err == nil {
			t.Fatalf("%v debió fallar", tc)
		}
	}
}

func TestDropPlan(t *testing.T) {
	ded, err := DropPlan("web", "web", "dedicated")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(ded, "\n")
	for _, want := range []string{"DROP OWNED BY", "DROP SCHEMA", "DROP ROLE"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %q: %v", want, ded)
		}
	}
	sh, err := DropPlan("web", SharedRole, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(sh) != 1 || !strings.Contains(sh[0], "DROP SCHEMA") {
		t.Fatalf("shared solo borra esquema: %v", sh)
	}
	for _, tc := range [][3]string{{"", "web", "dedicated"}, {"web", "MAL", "dedicated"}} {
		if _, err := DropPlan(tc[0], tc[1], tc[2]); err == nil {
			t.Fatalf("%v debió fallar", tc)
		}
	}
}

func TestSharedSearchPath(t *testing.T) {
	got := SharedSearchPathStmt("rkt", []string{"a", "b"})
	if got != `ALTER ROLE "rkt_apps" IN DATABASE "rkt" SET search_path = "a", "b", "public"` {
		t.Fatalf("got %q", got)
	}
	if got := SharedSearchPathStmt("rkt", nil); !strings.HasSuffix(got, `SET search_path = "public"`) {
		t.Fatalf("sin esquemas: %q", got)
	}
}

func TestDumpArgs(t *testing.T) {
	args := DumpArgs("rkt", "web", "w.dump")
	joined := strings.Join(args, " ")
	for _, want := range []string{"-Fc", "-n", "web", "rkt", "w.dump"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %q: %v", want, args)
		}
	}
}

func fakeBins(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if n == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, n+exeSuffix()), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveBinDir(t *testing.T) {
	dir := fakeBins(t, "initdb", "pg_ctl", "psql", "pg_dump", "pg_restore")
	setting := filepath.Join(dir, "pg_ctl"+exeSuffix())
	b, err := ResolveBin(setting)
	if err != nil {
		t.Fatal(err)
	}
	if b.PgCtl != setting || b.Initdb == "" || b.Psql == "" || b.PgDump == "" || b.PgRestore == "" {
		t.Fatalf("bins: %+v", b)
	}
	dir2 := fakeBins(t, "initdb", "psql")
	_, err = ResolveBin(filepath.Join(dir2, "initdb"+exeSuffix()))
	if err == nil || !strings.Contains(err.Error(), "pg_dump") {
		t.Fatalf("debió pedir pg_dump: %v", err)
	}
	b2, err := ResolveBin(filepath.Join(dir, "pg_ctl"+exeSuffix()))
	if err != nil {
		t.Fatal(err)
	}
	if b2.PgCtl == "" || b2.Initdb == "" {
		t.Fatalf("opcionales presentes deben resolverse: %+v", b2)
	}
}
