package pg

import (
	"strings"
	"testing"
)

func TestParseFilter(t *testing.T) {
	f, err := ParseFilter("email:ilike:%@x.com")
	if err != nil || f.Col != "email" || f.Op != "ilike" || f.Value != "%@x.com" {
		t.Fatalf("parse: %+v %v", f, err)
	}
	// El valor puede traer : (horas, URLs).
	f, err = ParseFilter("creado:gte:2024-01-01T10:00:00")
	if err != nil || f.Value != "2024-01-01T10:00:00" {
		t.Fatalf("valor con colon: %+v %v", f, err)
	}
	for _, bad := range []string{"", "sincolon", "a:b", "MAL:eq:1", "a:otro:1", "a:isnull:x", "a:in:"} {
		if _, err := ParseFilter(bad); err == nil {
			t.Fatalf("%q debió fallar", bad)
		}
	}
	if _, err := ParseFilter("a:isnull:"); err != nil {
		t.Fatalf("isnull sin valor: %v", err)
	}
}

func TestBuildSelectFiltrosOrden(t *testing.T) {
	q, args, err := BuildSelect("app", "t", RowsQuery{
		Limit: 10, Offset: 20, OrderCol: "id", OrderDir: "desc",
		Filters: []RowFilter{{Col: "edad", Op: "gte", Value: "18"}, {Col: "nick", Op: "isnotnull"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"app"."t"`, `"edad" >= $1`, `"nick" IS NOT NULL`, `ORDER BY "id" DESC`, `LIMIT $2 OFFSET $3`} {
		if !strings.Contains(q, want) {
			t.Fatalf("falta %q en %q", want, q)
		}
	}
	if len(args) != 3 || args[0] != "18" || args[1] != 10 || args[2] != 20 {
		t.Fatalf("args: %v", args)
	}
}

func TestBuildSelectBusqueda(t *testing.T) {
	q, args, err := BuildSelect("s", "t", RowsQuery{Limit: 5, Search: "ana"}, []string{"nombre", "email"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, `"nombre"::text ILIKE $1`) || !strings.Contains(q, `"email"::text ILIKE $1`) {
		t.Fatalf("búsqueda OR: %q", q)
	}
	if len(args) != 3 || args[0] != "%ana%" {
		t.Fatalf("args búsqueda: %v", args)
	}
}

func TestBuildSelectInvalido(t *testing.T) {
	if _, _, err := BuildSelect("MAL", "t", RowsQuery{}, nil); err == nil {
		t.Fatal("esquema inválido")
	}
	if _, _, err := BuildSelect("s", "t", RowsQuery{OrderCol: "x", OrderDir: "sideways"}, nil); err == nil {
		t.Fatal("dir inválida")
	}
}

func TestBuildInsert(t *testing.T) {
	q, args, err := BuildInsert("s", "t", map[string]any{"b": 1, "a": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, `INSERT INTO "s"."t" ("a", "b") VALUES ($1, $2) RETURNING *`) {
		t.Fatalf("insert: %q", q)
	}
	if len(args) != 2 || args[0] != "x" || args[1] != 1 {
		t.Fatalf("args ordenados: %v", args)
	}
	if _, _, err := BuildInsert("s", "t", nil); err == nil {
		t.Fatal("vacío debe fallar")
	}
	if _, _, err := BuildInsert("s", "t", map[string]any{"MAL": 1}); err == nil {
		t.Fatal("col inválida")
	}
}

func TestBuildUpdateDelete(t *testing.T) {
	q, args, err := BuildUpdate("s", "t", map[string]any{"id": 1}, map[string]any{"v": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, `UPDATE "s"."t" SET "v" = $1 WHERE "id" = $2`) {
		t.Fatalf("update: %q", q)
	}
	if len(args) != 2 {
		t.Fatalf("args: %v", args)
	}
	if _, _, err := BuildUpdate("s", "t", nil, map[string]any{"v": 1}); err == nil {
		t.Fatal("sin pk debe fallar")
	}
	dq, _, err := BuildDelete("s", "t", map[string]any{"id": 1}, nil)
	if err != nil || !strings.Contains(dq, `DELETE FROM "s"."t" WHERE "id" = $1`) {
		t.Fatalf("delete pk: %q %v", dq, err)
	}
	dq, dargs, err := BuildDelete("s", "t", nil, []RowFilter{{Col: "v", Op: "eq", Value: "x"}})
	if err != nil || !strings.Contains(dq, `"v" = $1`) || len(dargs) != 1 {
		t.Fatalf("delete filtros: %q %v %v", dq, dargs, err)
	}
	if _, _, err := BuildDelete("s", "t", nil, nil); err == nil {
		t.Fatal("sin where debe fallar")
	}
}

func TestBuildCreateTable(t *testing.T) {
	q, err := BuildCreateTable("s", "t", []ColumnDef{
		{Name: "id", Type: "uuid", PK: true, Default: "gen_random_uuid()"},
		{Name: "email", Type: "text", NotNull: true},
		{Name: "edad", Type: "integer", Default: "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`CREATE TABLE "s"."t"`, `"id" uuid NOT NULL DEFAULT gen_random_uuid()`, `"email" text NOT NULL`, `PRIMARY KEY ("id")`} {
		if !strings.Contains(q, want) {
			t.Fatalf("falta %q en:\n%s", want, q)
		}
	}
	for _, tc := range [][]ColumnDef{
		{},
		{{Name: "MAL", Type: "text"}},
		{{Name: "a", Type: "otro"}},
		{{Name: "a", Type: "text", Default: "DROP TABLE x"}},
		{{Name: "a", Type: "text"}, {Name: "a", Type: "int"}},
	} {
		if _, err := BuildCreateTable("s", "t", tc); err == nil {
			t.Fatalf("%v debió fallar", tc)
		}
	}
}

func TestValidColTypeDefault(t *testing.T) {
	for _, ok := range []string{"text", "varchar(255)", "numeric(10,2)", "uuid", "jsonb", "timestamptz", "double precision"} {
		if err := ValidColType(ok); err != nil {
			t.Fatalf("%q válido: %v", ok, err)
		}
	}
	for _, bad := range []string{"otro", "varchar(0)", "text; DROP", "enum('a')"} {
		if err := ValidColType(bad); err == nil {
			t.Fatalf("%q debió fallar", bad)
		}
	}
	for _, ok := range []string{"", "NULL", "0", "'hola'", "now()", "gen_random_uuid()", "true"} {
		if err := ValidColDefault(ok); err != nil {
			t.Fatalf("default %q: %v", ok, err)
		}
	}
	if err := ValidColDefault("pg_sleep(1)"); err == nil {
		t.Fatal("default arbitrario debe fallar")
	}
}
