package store

import "testing"

func TestGitCredsCRUD(t *testing.T) {
	db := newDB(t)
	if l, err := db.ListGitCreds(); err != nil || len(l) != 0 {
		t.Fatalf("vacío: %v %v", l, err)
	}
	v0 := db.ConfigVersion()
	if err := db.AddGitCred(GitCred{Name: "b", Kind: "pat", Match: "acme/*"}); err != nil {
		t.Fatal(err)
	}
	if err := db.AddGitCred(GitCred{Name: "a", Kind: "ssh", Match: "acme/web", PublicKey: "ssh-ed25519 AAA", Fingerprint: "SHA256:x"}); err != nil {
		t.Fatal(err)
	}
	if db.ConfigVersion() <= v0 {
		t.Fatal("alta debe subir la versión de config")
	}
	l, err := db.ListGitCreds()
	if err != nil || len(l) != 2 || l[0].Name != "a" || l[0].Fingerprint != "SHA256:x" || l[0].CreatedAt == "" {
		t.Fatalf("lista ordenada por nombre: %+v %v", l, err)
	}
	if err := db.AddGitCred(GitCred{Name: "a", Kind: "pat", Match: "*"}); err == nil {
		t.Fatal("nombre duplicado debe fallar")
	}
	if err := db.DeleteGitCred("a"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteGitCred("a"); err == nil {
		t.Fatal("borrar inexistente debe fallar")
	}
	if l, _ := db.ListGitCreds(); len(l) != 1 || l[0].Name != "b" {
		t.Fatalf("tras borrar: %+v", l)
	}
}
