package gitcred

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/store"
)

func TestPickMasEspecifico(t *testing.T) {
	creds := []store.GitCred{
		{Name: "todo", Kind: KindPAT, Match: "*"},
		{Name: "org", Kind: KindPAT, Match: "acme/*"},
		{Name: "deploy", Kind: KindSSH, Match: "acme/secreto"},
		{Name: "org-ssh", Kind: KindSSH, Match: "acme/*"},
	}
	cases := []struct{ owner, repo, want string }{
		{"acme", "secreto", "deploy"}, // repo exacto gana
		{"ACME", "web", "org"},        // owner/* y PAT antes que SSH; sin importar mayúsculas
		{"otro", "x", "todo"},         // comodín
	}
	for _, c := range cases {
		got := Pick(creds, c.owner, c.repo)
		if got == nil || got.Name != c.want {
			t.Errorf("%s/%s = %+v quiero %s", c.owner, c.repo, got, c.want)
		}
	}
	if Pick(creds[1:3], "otro", "x") != nil {
		t.Error("sin comodín no debe aplicar a otro owner")
	}
}

func TestValidaciones(t *testing.T) {
	for _, ok := range []string{"*", "acme/*", "acme/web"} {
		if err := ValidMatch(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "acme", "*/web", "a/b/c", "a b/c", "acme/"} {
		if ValidMatch(bad) == nil {
			t.Errorf("%q debía fallar", bad)
		}
	}
	for _, n := range []string{"ci", "mi-org.1"} {
		if !ValidName(n) {
			t.Errorf("nombre %q válido", n)
		}
	}
	for _, n := range []string{"", "../x", "A", "a/b", strings.Repeat("a", 49)} {
		if ValidName(n) {
			t.Errorf("nombre %q inválido", n)
		}
	}
	if ValidPAT("corto") == nil || ValidPAT("ghp_"+strings.Repeat("a", 36)) != nil || ValidPAT("ghp_aaaa aaaaaaaaaaaaaaaaaaaaaaaaaaa") == nil {
		t.Error("ValidPAT")
	}
}

func TestSecretosPermisosYRoundtrip(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSecret(dir, "ci", KindPAT, []byte("ghp_"+strings.Repeat("x", 36)+"\n")); err != nil {
		t.Fatal(err)
	}
	tok, err := ReadPAT(dir, "ci")
	if err != nil || !strings.HasPrefix(tok, "ghp_") || strings.Contains(tok, "\n") {
		t.Fatalf("read: %q %v", tok, err)
	}
	if err := WriteSecret(dir, "../escape", KindPAT, []byte("x")); err == nil {
		t.Fatal("nombre con ruta debe fallar")
	}
	if err := RemoveSecret(dir, "ci", KindPAT); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "git", "ci.pat")); !os.IsNotExist(err) {
		t.Fatal("secreto no borrado")
	}
	if err := RemoveSecret(dir, "ci", KindPAT); err != nil {
		t.Fatal("borrar dos veces es idempotente:", err)
	}
}

func TestDeployKey(t *testing.T) {
	priv, pub, fp, err := NewDeployKey("rkt-deploy")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") || !strings.HasSuffix(pub, " rkt-deploy") || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("pub=%q fp=%q", pub, fp)
	}
	pub2, fp2, err := InspectPrivateKey(priv)
	if err != nil || fp2 != fp || !strings.HasPrefix(pub, pub2) {
		t.Fatalf("inspect: %q %q %v", pub2, fp2, err)
	}
	if _, _, err := InspectPrivateKey([]byte("no es una llave")); err == nil {
		t.Fatal("basura debe fallar")
	}
	dir := t.TempDir()
	if err := WriteSecret(dir, "k1", KindSSH, priv); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(KeyPath(dir, "k1")); err != nil {
		t.Fatal(err)
	}
}
