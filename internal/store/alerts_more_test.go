package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAlertChannelsFiresDeliveries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := db.AddRuleFull("app", "latency_p95", 200, 15, true, 0)
	if err != nil || id == 0 {
		t.Fatalf("AddRuleFull: %v", err)
	}
	if err := db.UpdateRule(id, "app", "memory_mb", 128, 10, true, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddFire(id, 150, 128, "test"); err != nil {
		t.Fatal(err)
	}
	fires, err := db.ListFires(id, 10)
	if err != nil || len(fires) != 1 || fires[0].Value != 150 {
		t.Fatalf("fires = %+v %v", fires, err)
	}
	ch, err := db.AddChannel("ops", "https://example.com/hook", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateChannel(ch, "ops2", "https://example.com/hook2", true); err != nil {
		t.Fatal(err)
	}
	chans, err := db.ListChannels()
	if err != nil || len(chans) != 1 || chans[0].Name != "ops2" {
		t.Fatalf("channels = %+v %v", chans, err)
	}
	if _, err := db.EnqueueDelivery(fires[0].ID, ch); err != nil {
		t.Fatal(err)
	}
	pending, err := db.PendingDeliveries(10)
	if err != nil || len(pending) != 1 || pending[0].URL == "" {
		t.Fatalf("pending = %+v %v", pending, err)
	}
	if err := db.MarkDelivery(pending[0].ID, "sent", 1, "", ""); err != nil {
		t.Fatal(err)
	}
	if pending, _ = db.PendingDeliveries(10); len(pending) != 0 {
		t.Fatalf("entrega enviada sigue pendiente: %+v", pending)
	}
	if err := db.DeleteChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRule(id); err != nil {
		t.Fatal(err)
	}
}

func TestUsersSessionsAudit(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "u.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateUser("admin", "password-larga-1", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser("op", "password-larga-2", RoleOperator); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole("op", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserDisabled("op", true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CheckPassword("op", "password-larga-2"); err == nil {
		t.Fatal("usuario deshabilitado no debe entrar")
	}
	if err := db.SetUserDisabled("op", false); err != nil {
		t.Fatal(err)
	}
	tok, err := db.NewSession("op", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := db.ListSessions("op")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v %v", sessions, err)
	}
	if err := db.RevokeSession("op", sessions[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := db.SessionUser(tok); ok {
		t.Fatal("sesión revocada debe invalidarse")
	}
	if err := db.RevokeAllSessions("op"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateLastLogin("op"); err != nil {
		t.Fatal(err)
	}
	users, err := db.ListUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("users = %+v %v", users, err)
	}
	if err := db.AddAudit("admin", "test", "op", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	audit, err := db.ListAudit(10)
	if err != nil || len(audit) != 1 || audit[0].Action != "test" {
		t.Fatalf("audit = %+v %v", audit, err)
	}
}
