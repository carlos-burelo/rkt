package pg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCreateIndex(t *testing.T) {
	q, err := BuildCreateIndex("s", "t", "t_a_idx", []string{"a"}, false, "")
	if err != nil || !strings.Contains(q, `CREATE INDEX "t_a_idx" ON "s"."t" USING btree ("a")`) {
		t.Fatalf("got %q %v", q, err)
	}
	q, err = BuildCreateIndex("s", "t", "", []string{"a", "b"}, true, "gin")
	if err != nil || !strings.Contains(q, "UNIQUE") || !strings.Contains(q, "USING gin") {
		t.Fatalf("unique/gin: %q %v", q, err)
	}
	for _, tc := range []struct {
		s, tb, n string
		cols     []string
		m        string
	}{
		{"MAL", "t", "i", []string{"a"}, ""},
		{"s", "t", "i", nil, ""},
		{"s", "t", "MAL IDX!", []string{"a"}, ""},
		{"s", "t", "i", []string{"MAL"}, ""},
		{"s", "t", "i", []string{"a"}, "otro"},
	} {
		if _, err := BuildCreateIndex(tc.s, tc.tb, tc.n, tc.cols, false, tc.m); err == nil {
			t.Fatalf("%v debió fallar", tc)
		}
	}
	if _, err := BuildDropIndex("s", "t_a_idx"); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildDropIndex("s", "MAL IDX!"); err == nil {
		t.Fatal("drop inválido")
	}
}

func TestBuildPolicy(t *testing.T) {
	q, err := BuildCreatePolicy("s", "t", "lectores", "select", []string{"public"}, "true", "")
	if err != nil || !strings.Contains(q, `CREATE POLICY "lectores"`) || !strings.Contains(q, "FOR SELECT") {
		t.Fatalf("got %q %v", q, err)
	}
	q, err = BuildCreatePolicy("s", "t", "p", "all", []string{"app1"}, "auth.uid() = id", "true")
	if err != nil || !strings.Contains(q, `TO "app1"`) || !strings.Contains(q, "WITH CHECK") {
		t.Fatalf("roles/check: %q %v", q, err)
	}
	if _, err := BuildCreatePolicy("s", "t", "p", "otro", nil, "", ""); err == nil {
		t.Fatal("cmd inválido")
	}
	if _, err := BuildCreatePolicy("s", "t", "p", "select", nil, "a;b", ""); err == nil {
		t.Fatal("using con ; debe fallar")
	}
	if _, err := BuildDropPolicy("s", "t", "p"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAlter(t *testing.T) {
	if q, err := BuildRenameTable("s", "a", "b"); err != nil || !strings.Contains(q, `RENAME TO "b"`) {
		t.Fatalf("rename table: %q %v", q, err)
	}
	if _, err := BuildRenameTable("s", "a", "a"); err == nil {
		t.Fatal("igual debe fallar")
	}
	if q, err := BuildRenameColumn("s", "t", "a", "b"); err != nil || !strings.Contains(q, "RENAME COLUMN") {
		t.Fatalf("rename col: %q %v", q, err)
	}
	if q, err := BuildAlterColumnType("s", "t", "a", "uuid"); err != nil || !strings.Contains(q, `USING "a"::uuid`) {
		t.Fatalf("alter type: %q %v", q, err)
	}
	if q, err := BuildAlterColumnDefault("s", "t", "a", "now()", false); err != nil || !strings.Contains(q, "SET DEFAULT now()") {
		t.Fatalf("default: %q %v", q, err)
	}
	if q, err := BuildAlterColumnDefault("s", "t", "a", "", true); err != nil || !strings.Contains(q, "DROP DEFAULT") {
		t.Fatalf("drop default: %q %v", q, err)
	}
	if q, err := BuildAlterColumnNullable("s", "t", "a", true); err != nil || !strings.Contains(q, "SET NOT NULL") {
		t.Fatalf("not null: %q %v", q, err)
	}
	if q, err := BuildTruncate("s", "t"); err != nil || !strings.Contains(q, "TRUNCATE TABLE") {
		t.Fatalf("truncate: %q %v", q, err)
	}
	stmts, err := BuildDuplicate("s", "t", "t2", true)
	if err != nil || len(stmts) != 2 || !strings.Contains(stmts[0], "INCLUDING ALL") {
		t.Fatalf("dup: %v %v", stmts, err)
	}
	if _, err := BuildDuplicate("s", "t", "t", false); err == nil {
		t.Fatal("mismo nombre debe fallar")
	}
}

func TestBuildInsertMany(t *testing.T) {
	q, args, err := BuildInsertMany("s", "t", []string{"a", "b"}, [][]any{{1, "x"}, {2, "y"}})
	if err != nil || !strings.Contains(q, `INSERT INTO "s"."t" ("a", "b") VALUES ($1, $2), ($3, $4)`) || len(args) != 4 {
		t.Fatalf("got %q %v %v", q, args, err)
	}
	if _, _, err := BuildInsertMany("s", "t", []string{"a"}, [][]any{{1, 2}}); err == nil {
		t.Fatal("ancho distinto debe fallar")
	}
	if _, _, err := BuildInsertMany("s", "t", nil, [][]any{{1}}); err == nil {
		t.Fatal("sin cols debe fallar")
	}
}

func TestSplitStatements(t *testing.T) {
	got := SplitStatements(`SELECT 1; SELECT 'a;b'; -- c;d
SELECT 2`)
	if len(got) != 3 || !strings.Contains(got[1], "'a;b'") {
		t.Fatalf("split: %q", got)
	}
	if got := SplitStatements(`/* ; */ SELECT 1`); len(got) != 1 {
		t.Fatalf("block comment: %q", got)
	}
	if len(SplitStatements("  ;  ")) != 0 {
		t.Fatal("vacío")
	}
}

func TestListBackups(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"pg-app-20240101-000000.dump", "otro.txt", "pg-otro-20240102-000000.dump"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListBackups(dir, "app")
	if err != nil || len(got) != 1 || got[0].Name != "pg-app-20240101-000000.dump" {
		t.Fatalf("filtrado por app: %v %v", got, err)
	}
	if _, err := BackupPath(dir, "app", "../x.dump"); err == nil {
		t.Fatal("traversal debe fallar")
	}
	if err := DeleteBackup(dir, "app", "pg-app-20240101-000000.dump"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pg-app-20240101-000000.dump")); !os.IsNotExist(err) {
		t.Fatal("no borrado")
	}
}
