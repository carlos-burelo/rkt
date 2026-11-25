package pg

import (
	"strings"
	"testing"
)

func TestBuildCreateView(t *testing.T) {
	q, err := BuildCreateView("s", "v", "SELECT 1", false, false, false)
	if err != nil || !strings.Contains(q, `CREATE VIEW "s"."v" AS SELECT 1`) {
		t.Fatalf("got %q %v", q, err)
	}
	q, err = BuildCreateView("s", "m", "SELECT a FROM t", true, false, false)
	if err != nil || !strings.Contains(q, "MATERIALIZED VIEW") || !strings.Contains(q, "WITH NO DATA") {
		t.Fatalf("matview: %q %v", q, err)
	}
	q, err = BuildCreateView("s", "v", "SELECT 1", false, true, false)
	if err != nil || !strings.Contains(q, "OR REPLACE") {
		t.Fatalf("or replace: %q %v", q, err)
	}
	for _, tc := range []struct {
		name, sel string
		mat, rep  bool
	}{
		{"MAL", "SELECT 1", false, false},
		{"v", "DELETE FROM t", false, false},
		{"v", "SELECT 1; SELECT 2", false, false},
		{"v", "", false, false},
		{"v", "SELECT 1", true, true},
	} {
		if _, err := BuildCreateView("s", tc.name, tc.sel, tc.mat, tc.rep, false); err == nil {
			t.Fatalf("%v debió fallar", tc)
		}
	}
	if _, err := BuildDropView("s", "v", false); err != nil {
		t.Fatal(err)
	}
	if q, err := BuildRefreshMatview("s", "m", true); err != nil || !strings.Contains(q, "WITH DATA") {
		t.Fatalf("refresh: %q %v", q, err)
	}
}

func TestBuildRoutine(t *testing.T) {
	q, err := BuildDropRoutine("s", "f(integer)", false)
	if err != nil || !strings.Contains(q, `DROP FUNCTION "s"."f"(integer)`) {
		t.Fatalf("drop func: %q %v", q, err)
	}
	q, err = BuildDropRoutine("s", "p()", true)
	if err != nil || !strings.Contains(q, "DROP PROCEDURE") {
		t.Fatalf("drop proc: %q %v", q, err)
	}
	if _, err := BuildDropRoutine("s", "MAL", false); err == nil {
		t.Fatal("identidad mala debe fallar")
	}
	if _, err := BuildDropRoutine("MAL", "f()", false); err == nil {
		t.Fatal("esquema malo debe fallar")
	}
	q, err = BuildCallProcedure("s", "p(1, 'x')")
	if err != nil || !strings.Contains(q, `CALL "s"."p"(1, 'x')`) {
		t.Fatalf("call: %q %v", q, err)
	}
	if _, err := BuildCallProcedure("s", "p(1); SELECT"); err == nil {
		t.Fatal("call con ; debe fallar")
	}
}

func TestBuildTrigger(t *testing.T) {
	q, err := BuildCreateTrigger("s", "t", "tr", "before", []string{"insert", "update"}, "row", "audit_fn()", "")
	if err != nil || !strings.Contains(q, `CREATE TRIGGER "tr" BEFORE INSERT OR UPDATE`) {
		t.Fatalf("create: %q %v", q, err)
	}
	q, err = BuildCreateTrigger("s", "t", "tr", "after", []string{"delete"}, "", "s.f()", "old.id > 0")
	if err != nil || !strings.Contains(q, "WHEN (old.id > 0)") {
		t.Fatalf("when: %q %v", q, err)
	}
	for _, tc := range []struct {
		timing string
		events []string
		fn     string
	}{
		{"otro", []string{"insert"}, "f()"},
		{"before", nil, "f()"},
		{"before", []string{"otro"}, "f()"},
		{"before", []string{"insert", "insert"}, "f()"},
		{"before", []string{"insert"}, "no-func"},
		{"before", []string{"insert"}, "f(); DROP"},
	} {
		if _, err := BuildCreateTrigger("s", "t", "tr", tc.timing, tc.events, "row", tc.fn, ""); err == nil {
			t.Fatalf("%v debió fallar", tc)
		}
	}
	if _, err := BuildDropTrigger("s", "t", "tr"); err != nil {
		t.Fatal(err)
	}
	if q, err := BuildToggleTrigger("s", "t", "tr", false); err != nil || !strings.Contains(q, "DISABLE TRIGGER") {
		t.Fatalf("toggle: %q %v", q, err)
	}
}

func TestBuildSequence(t *testing.T) {
	if q, err := BuildRestartSequence("s", "q", nil); err != nil || !strings.Contains(q, "RESTART") {
		t.Fatalf("restart: %q %v", q, err)
	}
	n := int64(42)
	if q, err := BuildRestartSequence("s", "q", &n); err != nil || !strings.Contains(q, "WITH 42") {
		t.Fatalf("restart with: %q %v", q, err)
	}
	if q, err := BuildSetval("s", "q", 7, true); err != nil || !strings.Contains(q, "setval('s.q', 7, true)") {
		t.Fatalf("setval: %q %v", q, err)
	}
	if _, err := BuildSetval("MAL", "q", 1, true); err == nil {
		t.Fatal("esquema malo debe fallar")
	}
}
