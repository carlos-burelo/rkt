package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokensCRUD(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.CreateToken("", RoleViewer, time.Hour); err == nil {
		t.Fatal("nombre vacío")
	}
	if _, err := db.CreateToken("ci", "otro", time.Hour); err == nil {
		t.Fatal("rol inválido")
	}
	if _, err := db.CreateToken("ci", RoleViewer, 0); err == nil {
		t.Fatal("ttl inválido")
	}
	raw, err := db.CreateToken("ci", RoleOperator, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	list, err := db.ListTokens()
	if err != nil || len(list) != 1 || list[0].Name != "ci" {
		t.Fatalf("list = %+v %v", list, err)
	}
	if user, role, ok := db.TokenUser(raw); !ok || user != "ci" || role != RoleOperator {
		t.Fatalf("TokenUser = %q %q %v", user, role, ok)
	}
	if _, _, ok := db.TokenUser("rkt_noexiste"); ok {
		t.Fatal("token inválido")
	}
	// Expirado se borra.
	if _, err := db.sql.Exec(`UPDATE api_tokens SET expires_at=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := db.TokenUser(raw); ok {
		t.Fatal("token expirado")
	}
	raw2, _ := db.CreateToken("ci2", RoleViewer, time.Hour)
	list, _ = db.ListTokens()
	if len(list) != 1 {
		t.Fatalf("esperaba 1 token tras expirar: %+v", list)
	}
	if err := db.RevokeToken(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeToken(list[0].ID); err == nil {
		t.Fatal("revocar dos veces debe fallar")
	}
	if _, _, ok := db.TokenUser(raw2); ok {
		t.Fatal("token revocado")
	}
}

func TestBackupYCheck(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InsertEvent("died", "a", "m"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "backups", "copia.db")
	if err := db.Backup(dest); err != nil {
		t.Fatal(err)
	}
	if err := CheckFile(dest); err != nil {
		t.Fatalf("copia inválida: %v", err)
	}
	if err := CheckFile(filepath.Join(dir, "no.db")); err == nil {
		t.Fatal("archivo inexistente")
	}
	bad := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(bad, []byte("no sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckFile(bad); err == nil {
		t.Fatal("archivo corrupto debe fallar")
	}
	// Prune: viejo borra, nuevo queda, subdirectorio intacto.
	old := filepath.Join(dir, "backups", "viejo.db")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "backups", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	n, err := PruneBackups(filepath.Dir(dest), time.Now().Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune = %d %v", n, err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal("la copia nueva no debe borrarse")
	}
	if n, err := PruneBackups(filepath.Join(dir, "nada"), time.Now()); err != nil || n != 0 {
		t.Fatalf("dir inexistente = %d %v", n, err)
	}
}

func TestPurgeSessionsYSetup(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateUser("u", "password-larga-1", RoleViewer); err != nil {
		t.Fatal(err)
	}
	tok, err := db.NewSession("u", time.Hour)
	if err != nil || tok == "" {
		t.Fatal(err)
	}
	if _, _, ok := db.SessionUser(tok); !ok {
		t.Fatal("sesión válida")
	}
	if _, err := db.sql.Exec(`UPDATE sessions SET expires_at=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if n, err := db.PurgeSessions(); err != nil || n != 1 {
		t.Fatalf("purge = %d %v", n, err)
	}
	// Idle se purga.
	tok, _ = db.NewSession("u", time.Hour)
	if _, err := db.sql.Exec(`UPDATE sessions SET last_seen=? WHERE token_hash=?`,
		time.Now().Add(-2*SessionIdle).UTC().Format(time.RFC3339), TokenHash(tok)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := db.SessionUser(tok); ok {
		t.Fatal("sesión idle debe invalidarse")
	}
	// Setup states caducados.
	if _, err := db.NewSetupState(-time.Second); err != nil {
		t.Fatal(err)
	}
	if err := db.PruneSetupStates(); err != nil {
		t.Fatal(err)
	}
}

func TestCronRuns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := db.InsertCronRun("a", "job", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetCronRunLog(id, "log.txt"); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishCronRun(id, "ok", 0, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := db.ListCronRuns("a", "job", 10)
	if err != nil || len(runs) != 1 || runs[0].Status != "ok" || runs[0].LogPath != "log.txt" {
		t.Fatalf("runs = %+v %v", runs, err)
	}
	all, err := db.ListCronRuns("", "", 10)
	if err != nil || len(all) != 1 {
		t.Fatalf("todas = %+v %v", all, err)
	}
	if run, ok, err := db.CronRunByID(id); err != nil || !ok || run.ID != id {
		t.Fatalf("by id = %+v %v %v", run, ok, err)
	}
	if _, ok, err := db.CronRunByID(999); err != nil || ok {
		t.Fatalf("by id inexistente = %v %v", ok, err)
	}
	if run, ok, err := db.LastCronRun("a", "job"); err != nil || !ok || run.ID != id {
		t.Fatalf("last = %+v %v %v", run, ok, err)
	}
	if _, ok, err := db.LastCronRun("a", "nada"); err != nil || ok {
		t.Fatalf("last inexistente = %v %v", ok, err)
	}
}
