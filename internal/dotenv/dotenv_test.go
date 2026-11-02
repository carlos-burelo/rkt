package dotenv

import (
	"os"
	"path/filepath"
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
}
