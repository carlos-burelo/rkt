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
