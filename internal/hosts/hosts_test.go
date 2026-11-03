package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RKT_HOSTS_PATH", p)
	return p
}

func TestAddYRemoveConservaAjenas(t *testing.T) {
	p := setup(t, "127.0.0.1 localhost\n10.0.0.1 otro\n")
	if err := Add("coatl-in.test"); err != nil {
		t.Fatal(err)
	}
	if err := Add("coatl-in.test"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if strings.Count(string(data), "coatl-in.test") != 1 {
		t.Fatalf("add no idempotente:\n%s", data)
	}
	if !strings.Contains(string(data), "10.0.0.1 otro") {
		t.Fatal("se perdió una entrada ajena")
	}
	list, err := List()
	if err != nil || len(list) != 1 || list[0] != "coatl-in.test" {
		t.Fatalf("list = %v %v", list, err)
	}
	if err := Remove("coatl-in.test"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(p)
	if strings.Contains(string(data), "coatl-in.test") || !strings.Contains(string(data), "10.0.0.1 otro") {
		t.Fatalf("remove incorrecto:\n%s", data)
	}
}

func TestAddRechazaHostInvalido(t *testing.T) {
	setup(t, "127.0.0.1 localhost\n")
	for _, bad := range []string{"", "sinpunto", "con espacio.test"} {
		if err := Add(bad); err == nil {
			t.Fatalf("%q debía fallar", bad)
		}
	}
	if err := Add("https://ok.test/x"); err != nil {
		t.Fatalf("URL pegada debe normalizarse: %v", err)
	}
	list, _ := List()
	if len(list) != 1 || list[0] != "ok.test" {
		t.Fatalf("list = %v, quiero ok.test", list)
	}
}

func TestPathWindows(t *testing.T) {
	t.Setenv("RKT_HOSTS_PATH", "")
	t.Setenv("SystemRoot", `D:\Win`)
	if got := Path(); got != filepath.Join(`D:\Win`, "System32", "drivers", "etc", "hosts") {
		t.Fatalf("Path = %q", got)
	}
	t.Setenv("SystemRoot", "")
	if got := Path(); got != filepath.Join(`C:\Windows`, "System32", "drivers", "etc", "hosts") {
		t.Fatalf("Path default = %q", got)
	}
}

func TestErroresYBloqueCorrupto(t *testing.T) {
	// archivo inexistente
	t.Setenv("RKT_HOSTS_PATH", filepath.Join(t.TempDir(), "nope"))
	if _, err := List(); err == nil {
		t.Fatal("List sin archivo debe fallar")
	}
	if err := Add("a.test"); err == nil {
		t.Fatal("Add sin archivo debe fallar")
	}

	// host inválido
	setup(t, "")
	for _, h := range []string{"", "sindominio", "a b", "http://", "a/b"} {
		if err := Add(h); err == nil {
			t.Fatalf("host %q debe ser inválido", h)
		}
	}

	// bloque sin cierre = corrupto
	p := setup(t, "# rkt-managed begin (no editar a mano)\n127.0.0.1 a.test\n")
	if err := Remove("a.test"); err == nil || !strings.Contains(err.Error(), "corrupto") {
		t.Fatalf("bloque corrupto = %v", err)
	}
	// managedLines sin bloque / con comentarios
	if managedLines("nada") != nil {
		t.Fatal("sin bloque debe ser nil")
	}
	if got := managedLines("# rkt-managed begin (no editar a mano)\n\n# c\n127.0.0.1 a\n# rkt-managed end\n"); len(got) != 1 || got[0] != "127.0.0.1 a" {
		t.Fatalf("managedLines = %v", got)
	}
	_ = p
}

func TestRemoveBordesYEscrituraReadonly(t *testing.T) {
	p := setup(t, "127.0.0.1 localhost\n")
	if err := Add("a.test"); err != nil {
		t.Fatal(err)
	}
	if err := Add("b.test"); err != nil {
		t.Fatal(err)
	}
	if err := Remove("a.test"); err != nil {
		t.Fatal(err)
	}
	list, _ := List()
	if len(list) != 1 || list[0] != "b.test" {
		t.Fatalf("tras remove = %v", list)
	}
	if err := Remove("host-invalido"); err == nil {
		t.Fatal("Remove inválido debe fallar")
	}
	// archivo read-only: no puede reescribir
	if err := os.Chmod(p, 0o444); err != nil {
		t.Skipf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if err := Add("c.test"); err == nil {
		t.Fatal("escribir hosts read-only debe fallar")
	}
}
