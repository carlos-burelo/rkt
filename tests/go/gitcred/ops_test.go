package gitcred

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/store"
)

func newDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAddRemoveSSHGenerada(t *testing.T) {
	db, sec := newDB(t), t.TempDir()
	res, err := Add(db, sec, AddReq{Name: "deploy-web", Kind: KindSSH, Match: "acme/web"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.DeployKey, "ssh-ed25519 ") || res.Cred.Fingerprint == "" || res.Next == "" {
		t.Fatalf("resultado: %+v", res)
	}
	if _, err := os.Stat(KeyPath(sec, "deploy-web")); err != nil {
		t.Fatal("falta la privada:", err)
	}
	list, _ := db.ListGitCreds()
	if len(list) != 1 || list[0].Match != "acme/web" {
		t.Fatalf("lista: %+v", list)
	}
	if strings.Contains(list[0].PublicKey, "PRIVATE") {
		t.Fatal("la privada no debe estar en metadatos")
	}
	tok, key, err := Material(db, sec, "deploy-web")
	if err != nil || tok != "" || key != KeyPath(sec, "deploy-web") {
		t.Fatalf("access: %q %q %v", tok, key, err)
	}
	if err := Remove(db, sec, "deploy-web"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(KeyPath(sec, "deploy-web")); !os.IsNotExist(err) {
		t.Fatal("secreto sin borrar")
	}
	if err := Remove(db, sec, "deploy-web"); err == nil {
		t.Fatal("borrar inexistente debe fallar")
	}
}

func TestAddRechazaYNoDejaHuerfanos(t *testing.T) {
	db, sec := newDB(t), t.TempDir()
	pat := "ghp_" + strings.Repeat("a", 36)
	bad := []AddReq{
		{Name: "../x", Kind: KindPAT, Secret: pat},
		{Name: "ok", Kind: KindPAT, Secret: "corto"},
		{Name: "ok", Kind: "otro", Secret: pat},
		{Name: "ok", Kind: KindPAT, Match: "a/b/c", Secret: pat},
		{Name: "ok", Kind: KindSSH, Secret: "no es una llave"},
	}
	for i, r := range bad {
		if _, err := Add(db, sec, r); err == nil {
			t.Errorf("caso %d debía fallar: %+v", i, r)
		}
	}
	if _, err := Add(db, sec, AddReq{Name: "ci", Kind: KindPAT, Secret: pat}); err != nil {
		t.Fatal(err)
	}
	// nombre duplicado: falla y NO pisa el secreto existente
	if _, err := Add(db, sec, AddReq{Name: "ci", Kind: KindPAT, Secret: "ghp_" + strings.Repeat("b", 36)}); err == nil {
		t.Fatal("duplicado debe fallar")
	}
	if tok, _ := ReadPAT(sec, "ci"); !strings.Contains(tok, "aaaa") {
		t.Fatalf("un alta duplicada no debe tocar el secreto original: %q", tok)
	}
}

func TestUserPATPrefiereComodin(t *testing.T) {
	db, sec := newDB(t), t.TempDir()
	if UserPAT(db, sec) != "" {
		t.Fatal("sin PAT debe ser vacío")
	}
	Add(db, sec, AddReq{Name: "org", Kind: KindPAT, Match: "acme/*", Secret: "ghp_" + strings.Repeat("o", 36)})
	Add(db, sec, AddReq{Name: "todo", Kind: KindPAT, Match: "*", Secret: "ghp_" + strings.Repeat("t", 36)})
	if got := UserPAT(db, sec); !strings.Contains(got, "ttt") {
		t.Fatalf("debe preferir el comodín: %q", got)
	}
}
