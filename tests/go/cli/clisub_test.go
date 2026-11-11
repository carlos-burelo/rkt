package cli

// CLI por subproceso: el binario real contra config temporal.
// Los 151 log.Fatal de manage.go lo hacen inmatable en proceso; por
// subproceso se prueban salidas, códigos y redondeles de verdad
// (users, backup/restore, apps externas, settings, export/import).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var cliBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rkt-cli-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, "tmp:", err)
		os.Exit(1)
	}
	exe := filepath.Join(dir, "rkt-test")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	// Raíz del repo: subir hasta go.mod.
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "raíz:", err)
		os.Exit(1)
	}
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/rkt-proxy")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build rkt: %v\n%s", err, out)
		os.Exit(1)
	}
	cliBin = exe
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func repoRoot() (string, error) {
	d, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("go.mod no encontrado desde %s", d)
		}
		d = parent
	}
}

func runCLI(t *testing.T, cfgPath, stdin string, args ...string) (int, string) {
	t.Helper()
	full := append([]string{"-config", cfgPath}, args...)
	cmd := exec.Command(cliBin, full...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	code := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("ejecutar %v: %v", args, err)
		}
	}
	return code, buf.String()
}

func freshCfg(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "rkt.yaml")
}

func TestCLIVersion(t *testing.T) {
	code, out := runCLI(t, freshCfg(t), "", "version")
	if code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("version: code=%d out=%q", code, out)
	}
}

func TestCLIUsersRoundTrip(t *testing.T) {
	cfg := freshCfg(t)
	if code, out := runCLI(t, cfg, "secreto-largo-123\n", "users", "add", "ana", "--role", "admin", "--password-stdin"); code != 0 || !strings.Contains(out, "creado") {
		t.Fatalf("add: code=%d out=%q", code, out)
	}
	if code, out := runCLI(t, cfg, "otro-secreto-456\n", "users", "add", "beto", "--password-stdin"); code != 0 {
		t.Fatalf("add2: code=%d out=%q", code, out)
	}
	_, out := runCLI(t, cfg, "", "users", "list")
	if !strings.Contains(out, "ana\tadmin") || !strings.Contains(out, "beto\tviewer") {
		t.Fatalf("list: %q", out)
	}
	if code, out := runCLI(t, cfg, "", "users", "del", "beto"); code != 0 || !strings.Contains(out, "borrado") {
		t.Fatalf("del: code=%d out=%q", code, out)
	}
	if _, out := runCLI(t, cfg, "", "users", "list"); strings.Contains(out, "beto") {
		t.Fatalf("beto sigue listado: %q", out)
	}
}

func TestCLIBackupRestore(t *testing.T) {
	cfg := freshCfg(t)
	add := func(name string) {
		t.Helper()
		if code, out := runCLI(t, cfg, "secreto-largo-123\n", "users", "add", name, "--role", "admin", "--password-stdin"); code != 0 {
			t.Fatalf("add %s: code=%d out=%q", name, code, out)
		}
	}
	add("ana")
	add("beto")
	dest := filepath.Join(t.TempDir(), "copia.db")
	if code, out := runCLI(t, cfg, "", "backup", dest); code != 0 || !strings.Contains(out, "backup ok") {
		t.Fatalf("backup: code=%d out=%q", code, out)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("backup no creado: %v", err)
	}
	if code, _ := runCLI(t, cfg, "", "users", "del", "ana"); code != 0 {
		t.Fatal("del previo al restore")
	}
	if code, out := runCLI(t, cfg, "", "restore", dest); code != 0 || !strings.Contains(out, "restore ok") {
		t.Fatalf("restore: code=%d out=%q", code, out)
	}
	if _, out := runCLI(t, cfg, "", "users", "list"); !strings.Contains(out, "ana\tadmin") {
		t.Fatalf("ana no volvió con el restore: %q", out)
	}
}

func TestCLIAppsExternalRoundTrip(t *testing.T) {
	cfg := freshCfg(t)
	dir := t.TempDir()
	if code, out := runCLI(t, cfg, "", "apps", "add", "--external", "--name", "ext", "--dir", dir, "--domain", "ext.test"); code != 0 {
		t.Fatalf("add: code=%d out=%q", code, out)
	}
	code, out := runCLI(t, cfg, "", "apps", "list", "--json")
	if code != 0 {
		t.Fatalf("list: code=%d out=%q", code, out)
	}
	var got []map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got) != 1 || got[0]["name"] != "ext" {
		t.Fatalf("list json = %q err=%v", out, err)
	}
	if code, out := runCLI(t, cfg, "", "apps", "rm", "ext"); code != 0 {
		t.Fatalf("rm: code=%d out=%q", code, out)
	}
	if code, out := runCLI(t, cfg, "", "apps", "list", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("list vacío: code=%d out=%q", code, out)
	}
}

func TestCLISettingsExportImport(t *testing.T) {
	cfg := freshCfg(t)
	if code, out := runCLI(t, cfg, "", "settings", "set", "base_domain=ej.test"); code != 0 {
		t.Fatalf("set: code=%d out=%q", code, out)
	}
	out := filepath.Join(t.TempDir(), "cfg.json")
	if code, msg := runCLI(t, cfg, "", "config", "export", out); code != 0 {
		t.Fatalf("export: code=%d out=%q", code, msg)
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(data), "ej.test") {
		t.Fatalf("export sin el valor: %v %q", err, data)
	}
	if code, _ := runCLI(t, cfg, "", "settings", "set", "base_domain=otro.test"); code != 0 {
		t.Fatal("set previo al import")
	}
	if code, msg := runCLI(t, cfg, "", "config", "import", out); code != 0 {
		t.Fatalf("import: code=%d out=%q", code, msg)
	}
	if code, msg := runCLI(t, cfg, "", "settings", "list"); code != 0 || !strings.Contains(msg, "base_domain=ej.test") {
		t.Fatalf("valor no restaurado: code=%d out=%q", code, msg)
	}
}

func TestCLIDeploymentsVacio(t *testing.T) {
	code, out := runCLI(t, freshCfg(t), "", "deployments", "list", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Fatalf("deployments vacío: code=%d out=%q", code, out)
	}
}

func TestCLIDoctorSinConfig(t *testing.T) {
	// Sin YAML el doctor falla explicando (no peta en seco).
	code, out := runCLI(t, freshCfg(t), "", "doctor")
	if code == 0 || !strings.Contains(out, "config") {
		t.Fatalf("doctor sin config: code=%d out=%q", code, out)
	}
}
