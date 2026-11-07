package dotenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	m, err := Parse(`
# comentario
export FOO=bar
BAZ="con espacios"
QUX='simple'
EMPTY=
SOLA
NORM=  recortada
`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"FOO": "bar", "BAZ": "con espacios", "QUX": "simple",
		"EMPTY": "", "SOLA": "", "NORM": "recortada",
	}
	for k, v := range want {
		if m[k] != v {
			t.Fatalf("%s: got %q quiere %q", k, m[k], v)
		}
	}
}

func TestParseBad(t *testing.T) {
	for _, body := range []string{"9MAL=x", "CON ESPACIO=x", "SINVALOR!?=x"} {
		if _, err := Parse(body); err == nil {
			t.Fatalf("%q debió fallar", body)
		}
	}
}

func TestParseFileMissing(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Fatal("inexistente debe fallar")
	}
}

func TestParseFileOK(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.env")
	os.WriteFile(p, []byte("A=1\n"), 0o644)
	m, err := ParseFile(p)
	if err != nil || m["A"] != "1" {
		t.Fatalf("got %v %v", m, err)
	}
}

func TestExpand(t *testing.T) {
	os.Setenv("RKT_DOT_OS", "osval")
	defer os.Unsetenv("RKT_DOT_OS")
	out := Expand(map[string]string{
		"A": "${RKT_DOT_OS}-x",
		"B": "$RKT_DOT_OS/y",
		"C": "suelto $ y ${SIN_CIERRE",
		"D": "${NOEXISTE}-vacio",
		"E": "$A!$RKT_DOT_OS",
	}, EnvironMap())
	if out["A"] != "osval-x" || out["B"] != "osval/y" {
		t.Fatalf("básico mal: %v", out)
	}
	if out["C"] != "suelto $ y ${SIN_CIERRE" || out["D"] != "-vacio" {
		t.Fatalf("bordes mal: %v", out)
	}
	if out["E"] != "osval-x!osval" {
		t.Fatalf("ordenado mal (E ve A inline): %q", out["E"])
	}
	if got := Expand(map[string]string{"P": "pa$$word"}, nil)["P"]; got != "pa$word" {
		t.Fatalf("escape $$ mal: %q", got)
	}
}

func TestParseErroresYExportSolo(t *testing.T) {
	if _, err := Parse("1MAL=x\n"); err == nil {
		t.Fatal("clave que empieza con número debe fallar")
	}
	if _, err := Parse("BA D=x\n"); err == nil {
		t.Fatal("clave con espacio debe fallar")
	}
	m, err := Parse("export\n\nexport OK=1\n")
	if err != nil || m["OK"] != "1" {
		t.Fatalf("export solo debe saltarse: %+v %v", m, err)
	}
	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("ParseFile inexistente debe fallar")
	}
}

func TestEnvironListYExpandBordes(t *testing.T) {
	t.Setenv("RKT_DOTENV_TEST", "base")
	if t.Setenv("GITHUB_TOKEN", "no-debe-pasar"); true {
		list := EnvironListApp(map[string]string{"APP_VAR": "1"})
		joined := strings.Join(list, "\n")
		if strings.Contains(joined, "GITHUB_TOKEN") || strings.Contains(joined, "no-debe-pasar") {
			t.Fatalf("secreto de máquina heredado: %v", list)
		}
		if !strings.Contains(joined, "APP_VAR=1") {
			t.Fatalf("falta env propio: %v", list)
		}
		if !strings.Contains(joined, "npm_config_verify_deps_before_run=false") {
			t.Fatalf("falta guard de pnpm: %v", list)
		}
	}

	list := EnvironList(map[string]string{"RKT_DOTENV_TEST": "override", "OTRA": "x"})
	seen := map[string]bool{}
	for _, kv := range list {
		if kv == "RKT_DOTENV_TEST=override" {
			seen["override"] = true
		}
		if kv == "OTRA=x" {
			seen["otra"] = true
		}
	}
	if !seen["override"] || !seen["otra"] {
		t.Fatalf("EnvironList no aplicó overrides: %v", seen)
	}

	cases := map[string]string{
		"$":      "$",    // $ suelto al final
		"$1":     "",     // $ seguido de dígito no forma variable
		"${SIN}": "",     // indefinida = ""
		"${AB":   "${AB", // llave sin cerrar se deja literal
		"a$":     "a$",
		"${V}x":  "valx",
		"A$V.B":  "Aval.B",
	}
	out := Expand(map[string]string{
		"K1": cases["$"],
		"K2": cases["$1"],
		"K3": cases["${SIN}"],
		"K4": cases["${AB"],
		"K5": cases["a$"],
		"K6": cases["${V}x"],
		"K7": cases["A$V.B"],
	}, map[string]string{"V": "val"})
	want := map[string]string{"K1": "$", "K2": "", "K3": "", "K4": "${AB", "K5": "a$", "K6": "valx", "K7": "Aval.B"}
	for k, v := range want {
		if out[k] != v {
			t.Fatalf("%s = %q, quiere %q", k, out[k], v)
		}
	}
}

func TestParseClavesValidas(t *testing.T) {
	m, err := Parse("A1=x\n_B=2\n")
	if err != nil || m["A1"] != "x" || m["_B"] != "2" {
		t.Fatalf("claves válidas: %+v %v", m, err)
	}
	if _, err := Parse("A-B=x\n"); err == nil {
		t.Fatal("guion en clave debe fallar")
	}
	if validKey("") {
		t.Fatal("clave vacía debe ser inválida")
	}
	if !validKey("A1") || !validKey("_B") {
		t.Fatal("claves con dígito/guion bajo deben ser válidas")
	}
}
