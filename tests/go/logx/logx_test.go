package logx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.log")
	r, err := NewRotating(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("12345678")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("abcdefgh")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatalf("no rotó: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err) // idempotente
	}
	if _, err := r.Write([]byte("x")); err == nil {
		t.Fatal("write tras close debe fallar")
	}
	// Abrir sobre un archivo grande rota al arrancar.
	if err := os.WriteFile(p, make([]byte, 20), 0o644); err != nil {
		t.Fatal(err)
	}
	r2, err := NewRotating(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	r2.Close()
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatalf("rotación al abrir: %v", err)
	}
	// Padre que es archivo: MkdirAll falla.
	padre := filepath.Join(dir, "archivo2")
	if err := os.WriteFile(padre, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRotating(filepath.Join(padre, "x.log"), 10); err == nil {
		t.Fatal("MkdirAll sobre archivo debe fallar")
	}
	// max<=0 usa default y un path directorio falla.
	r3, err := NewRotating(filepath.Join(dir, "sub", "b.log"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r3.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := r3.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRotating(dir, 10); err == nil {
		t.Fatal("abrir un directorio como log debe fallar")
	}
}

func TestDirOf(t *testing.T) {
	if dirOf(`C:\a\b.log`) != `C:\a` || dirOf("/a/b.log") != "/a" || dirOf("b.log") != "." {
		t.Fatal("dirOf mal")
	}
}

func TestControlTokenErrores(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "archivo")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureControlToken(filepath.Join(file, "rkt.yaml")); err == nil {
		t.Fatal("dir de logs sobre archivo debe fallar")
	}
	cfg := filepath.Join(dir, "rkt.yaml")
	path := ControlTokenPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corto\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := EnsureControlToken(cfg)
	if err != nil || len(tok) < 32 {
		t.Fatalf("token corto debe regenerarse: %q %v", tok, err)
	}
	// WriteFile falla si el path del token es un directorio.
	cfg2 := filepath.Join(dir, "otro", "rkt.yaml")
	p2 := ControlTokenPath(cfg2)
	if err := os.MkdirAll(filepath.Dir(p2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p2, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureControlToken(cfg2); err == nil {
		t.Fatal("WriteFile sobre directorio debe fallar")
	}
}

func TestControlTokenPersistente(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.yaml")
	t1, err := EnsureControlToken(cfg)
	if err != nil || len(t1) < 32 {
		t.Fatalf("crear token: %q %v", t1, err)
	}
	t2, err := EnsureControlToken(cfg)
	if err != nil || t2 != t1 {
		t.Fatalf("token no persistente: %q != %q (%v)", t2, t1, err)
	}
}

// TestRutasCanonicas: un solo hogar. DB+backups en data/, secretos en
// secrets/, logs operativos en logs/, apps y certs bajo el home.
func TestRutasCanonicas(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.local.yaml")
	home := HomeDir(cfg)
	if dir := DirFor(cfg); dir != filepath.Join(home, "logs") {
		t.Fatalf("DirFor = %q", dir)
	}
	if got := DBPath(cfg); got != filepath.Join(home, "data", "rkt.db") {
		t.Fatalf("db = %q", got)
	}
	if got := DataDir(cfg); got != filepath.Join(home, "data") {
		t.Fatalf("data = %q", got)
	}
	if got := SecretsDir(cfg); got != filepath.Join(home, "secrets") {
		t.Fatalf("secrets = %q", got)
	}
	cases := map[string]string{
		"log":    Path(cfg),
		"access": AccessPath(cfg),
	}
	for name, got := range cases {
		if filepath.Dir(got) != filepath.Join(home, "logs") {
			t.Fatalf("%s fuera de logs: %q", name, got)
		}
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
	if AppKeyPath(filepath.Join(dir, "config.yaml")) != filepath.Join(dir, "secrets", "github-app.pem") {
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
