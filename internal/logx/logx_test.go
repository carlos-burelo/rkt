package logx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRutasCanonicas: db, log operativo y access log viven bajo logs/.
func TestRutasCanonicas(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.local.yaml")
	dir := DirFor(cfg)
	if filepath.Base(dir) != "logs" {
		t.Fatalf("DirFor = %q", dir)
	}
	cases := map[string]string{
		"db":     DBPath(cfg),
		"log":    Path(cfg),
		"access": AccessPath(cfg),
	}
	for name, got := range cases {
		if filepath.Dir(got) != dir {
			t.Fatalf("%s fuera de logs: %q", name, got)
		}
	}
	if DBPath(cfg) != filepath.Join(dir, "rkt.db") {
		t.Fatalf("db = %q", DBPath(cfg))
	}
	if !strings.HasSuffix(AccessPath(cfg), "access.log") {
		t.Fatalf("access = %q", AccessPath(cfg))
	}
}

func TestAbsYFileExists(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Abs(p); got != p {
		t.Fatalf("Abs(absoluto) = %q", got)
	}
	if !fileExists(p) {
		t.Fatal("archivo existente debe dar true")
	}
	if fileExists(dir) {
		t.Fatal("directorio no cuenta como archivo")
	}
	if fileExists(filepath.Join(dir, "nope")) {
		t.Fatal("inexistente debe dar false")
	}
	// relativo sin archivo: se ancla al CWD
	wd, _ := os.Getwd()
	rel := Abs("no-existe-xyz.yaml")
	if rel != filepath.Join(wd, "no-existe-xyz.yaml") {
		t.Fatalf("Abs(relativo) = %q", rel)
	}
	if AppKeyPath(filepath.Join(dir, "config.yaml")) != filepath.Join(dir, "logs", "github-app.pem") {
		t.Fatalf("AppKeyPath = %q", AppKeyPath(filepath.Join(dir, "config.yaml")))
	}
}

func TestTailYStripANSI(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "log.txt")
	lines := []string{"uno", "\x1b[32mdos\x1b[0m", "tres", "cuatro"}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Tail(p, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "tres\ncuatro" {
		t.Fatalf("Tail(2) = %q", got)
	}
	// n<1 => 50, y limpia ANSI
	got, err = Tail(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\x1b") || !strings.Contains(got, "dos") {
		t.Fatalf("Tail(0) sin limpiar: %q", got)
	}
	if _, err := Tail(filepath.Join(dir, "nope"), 5); err == nil {
		t.Fatal("archivo inexistente debe fallar")
	}
	if StripANSI("\x1b[31mrojo\x1b[0m") != "rojo" {
		t.Fatal("StripANSI mal")
	}
}

func TestSetupEscribeYRoja(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rkt.local.yaml")
	logger, closeFn, err := Setup(cfg)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hola", "k", "v")
	closeFn()
	data, err := os.ReadFile(Path(cfg))
	if err != nil || !strings.Contains(string(data), "hola") {
		t.Fatalf("log = %q %v", data, err)
	}
	// si supera maxBytes, rota al abrir
	big := make([]byte, maxBytes+1)
	if err := os.WriteFile(Path(cfg), big, 0o644); err != nil {
		t.Fatal(err)
	}
	_, close2, err := Setup(cfg)
	if err != nil {
		t.Fatal(err)
	}
	close2()
	if _, err := os.Stat(Path(cfg) + ".1"); err != nil {
		t.Fatalf("falta rotación .1: %v", err)
	}
}

func TestAbsJuntoAlExeYSetupError(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("sin executable")
	}
	name := "rkt-abs-probe-unico.yaml"
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), name), []byte("x"), 0o644); err != nil {
		t.Skipf("no puedo escribir junto al exe: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(filepath.Join(filepath.Dir(exe), name)) })
	if got := Abs(name); got != filepath.Join(filepath.Dir(exe), name) {
		t.Fatalf("Abs junto al exe = %q", got)
	}

	file := filepath.Join(t.TempDir(), "archivo")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Setup(filepath.Join(file, "rkt.yaml")); err == nil {
		t.Fatal("Setup bajo un archivo debe fallar al crear logs/")
	}
}

func TestAbsCWDYFallbackTotal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cfg.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if got := Abs("cfg.yaml"); got != filepath.Join(dir, "cfg.yaml") {
		t.Fatalf("Abs CWD existente = %q", got)
	}

	oldGet, oldExe := getwd, executable
	getwd = func() (string, error) { return "", errors.New("sin cwd") }
	executable = func() (string, error) { return "", errors.New("sin exe") }
	t.Cleanup(func() { getwd, executable = oldGet, oldExe })
	if got := Abs("rel.yaml"); got != "rel.yaml" {
		t.Fatalf("fallback total = %q", got)
	}
}

func TestSetupLogEsDirectorio(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.local.yaml")
	if err := os.MkdirAll(DirFor(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(Path(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Setup(cfg); err == nil {
		t.Fatal("rkt.log como directorio debe fallar al abrir")
	}
}
