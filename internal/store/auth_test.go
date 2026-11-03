package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUsersSessions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if n, _ := db.CountUsers(); n != 0 {
		t.Fatalf("quiere 0, got %d", n)
	}
	if err := db.CreateUser("admin", "password-larga-1", RoleAdmin); err != nil {
		t.Fatalf("alta: %v", err)
	}
	if err := db.CreateUser("admin", "password-larga-2", RoleAdmin); err == nil {
		t.Fatal("duplicado debe fallar")
	}
	if err := db.CreateUser("x", "corta", RoleViewer); err == nil {
		t.Fatal("password corta debe fallar")
	}
	if err := db.CreateUser("MALO!", "password-larga-1", RoleViewer); err == nil {
		t.Fatal("nombre inválido debe fallar")
	}
	if err := db.CreateUser("v", "password-larga-1", "jefe"); err == nil {
		t.Fatal("rol inválido debe fallar")
	}
	if _, err := db.CheckPassword("admin", "mal"); err == nil {
		t.Fatal("password mal debe fallar")
	}
	if _, err := db.CheckPassword("nadie", "password-larga-1"); err == nil {
		t.Fatal("usuario inexistente debe fallar")
	}
	role, err := db.CheckPassword("admin", "password-larga-1")
	if err != nil || role != RoleAdmin {
		t.Fatalf("login: %v %q", err, role)
	}

	tok, err := db.NewSession("admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if u, r, ok := db.SessionUser(tok); !ok || u != "admin" || r != RoleAdmin {
		t.Fatalf("sesión: %q %q %v", u, r, ok)
	}
	if _, _, ok := db.SessionUser("falso"); ok {
		t.Fatal("token falso debe fallar")
	}
	// expirada se purga
	if _, err := db.NewSession("admin", -time.Second); err != nil {
		t.Fatal(err)
	}
	db.DeleteSession(tok)
	if _, _, ok := db.SessionUser(tok); ok {
		t.Fatal("logout debe invalidar")
	}

	// último admin no se borra
	if err := db.DeleteUser("admin"); err == nil {
		t.Fatal("último admin debe protegerse")
	}
	if err := db.CreateUser("admin2", "password-larga-2", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser("admin"); err != nil {
		t.Fatalf("con 2 admins sí: %v", err)
	}
	users, err := db.ListUsers()
	if err != nil || len(users) != 1 || users[0].Name != "admin2" {
		t.Fatalf("list: %+v %v", users, err)
	}
}
