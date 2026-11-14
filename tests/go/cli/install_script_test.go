package cli

// Instalador Windows (scripts/install.ps1): sintaxis real vía Parser de
// pwsh, pasos del borrado destructivo en orden (antes de la idempotencia),
// guardia Test-RktNukeDir ejecutada por tabla y los 2 comandos del README
// (primera vez + destructivo) con parseo de una línea. Sin pwsh se salta
// lo que lo exige (CI Linux); el resto es estático.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func needPwsh(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("sin pwsh en PATH")
	}
	return p
}

func readInstallPs1(t *testing.T) (root, body string) {
	t.Helper()
	r, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(r, "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	return r, string(b)
}

// pwshParseErrs parsea archivo o fragmento con el Parser real de PowerShell.
func pwshParseErrs(t *testing.T, pwsh, target string, isFile bool) []string {
	t.Helper()
	call := "ParseInput('" + target + "',[ref]$toks,[ref]$errs)"
	if isFile {
		call = "ParseFile('" + target + "',[ref]$toks,[ref]$errs)"
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-Command",
		"$errs=$null;$toks=$null;[void][System.Management.Automation.Language.Parser]::"+
			call+";$errs|ForEach-Object{$_.Message}").CombinedOutput()
	if err != nil {
		t.Fatalf("pwsh Parser: %v\n%s", err, out)
	}
	var errs []string
	for _, ln := range strings.Split(string(out), "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			errs = append(errs, ln)
		}
	}
	return errs
}

func TestInstallPs1Sintaxis(t *testing.T) {
	pwsh := needPwsh(t)
	root, _ := readInstallPs1(t)
	if errs := pwshParseErrs(t, pwsh, filepath.Join(root, "scripts", "install.ps1"), true); len(errs) > 0 {
		t.Fatalf("install.ps1 con errores de sintaxis:\n%s", strings.Join(errs, "\n"))
	}
}

func TestInstallPs1DestructivoOrdenYPasos(t *testing.T) {
	_, body := readInstallPs1(t)
	for _, s := range []string{
		"[switch]$Destructivo",
		"function Test-RktNukeDir",
		"if ($Destructivo)",
		"InstallDir no borrable",
		"Stop-Service Rkt",
		"Get-Service Rkt",
		"& $exe -config $cfg uninstall",
		"sc.exe delete Rkt",
		"Get-Process",
		"Stop-Process",
		"takeown",
		"/reset",
		`"RKT HTTP/HTTPS"`,
		`"RKT Dashboard"`,
		`"RKT Proxy HTTP/HTTPS"`,
		"RKT_GITHUB_TOKEN_FILE",
		"Remove-Item",
		"-Recurse -Force",
		"No pude borrar",
		"Limpieza destructiva OK",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("install.ps1 sin paso %q", s)
		}
	}
	wipe := strings.Index(body, "if ($Destructivo)")
	idem := strings.Index(body, "Ya instalado")
	if wipe < 0 || idem < 0 || wipe > idem {
		t.Errorf("el borrado debe ir antes de la idempotencia (wipe=%d idem=%d)", wipe, idem)
	}
	for _, b := range []string{`Program Files`, `Program Files (x86)`, `Windows`, `Users`} {
		if !strings.Contains(body, b) {
			t.Errorf("guardia sin veto a %q", b)
		}
	}
}

// extractPsFunction recorta la función pwsh con su nombre (llaves por
// profundidad; el cuerpo no mete llaves en strings).
func extractPsFunction(t *testing.T, body, name string) string {
	t.Helper()
	start := strings.Index(body, "function "+name)
	if start < 0 {
		t.Fatalf("función %s no encontrada", name)
	}
	depth, opened := 0, false
	for i, c := range body[start:] {
		switch c {
		case '{':
			depth++
			opened = true
		case '}':
			depth--
		}
		if opened && depth == 0 {
			return body[start : start+i+1]
		}
	}
	t.Fatalf("cierre de %s no encontrado", name)
	return ""
}

func TestInstallPs1NukeDirGuarda(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("instalador Windows-only")
	}
	pwsh := needPwsh(t)
	_, body := readInstallPs1(t)
	fn := extractPsFunction(t, body, "Test-RktNukeDir")
	cases := []struct{ in, want string }{
		{`C:\`, "False"},        // raíz: nunca
		{`C:\Windows`, "False"}, // sistema
		{`c:\windows`, "False"}, // insensible a mayúsculas
		{`C:\Program Files`, "False"},
		{`C:\Program Files (x86)`, "False"},
		{`C:\Users`, "False"},
		{``, "False"},                          // vacío
		{`C:\Program Files\rkt-proxy`, "True"}, // defecto
		{`C:\rkt\datos`, "True"},               // corto pero válido
		{`D:\apps\rkt`, "True"},                // otra unidad
	}
	var sb strings.Builder
	sb.WriteString(fn + "\n")
	for _, c := range cases {
		sb.WriteString("[Console]::WriteLine((Test-RktNukeDir '" + c.in + "'))\n")
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-Command", sb.String()).CombinedOutput()
	if err != nil {
		t.Fatalf("pwsh guardia: %v\n%s", err, out)
	}
	var got []string
	for _, ln := range strings.Split(string(out), "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			got = append(got, ln)
		}
	}
	if len(got) != len(cases) {
		t.Fatalf("guardia devolvió %d líneas, quieren %d:\n%s", len(got), len(cases), out)
	}
	for i, c := range cases {
		if got[i] != c.want {
			t.Errorf("Test-RktNukeDir(%q) = %s, quiere %s", c.in, got[i], c.want)
		}
	}
}

func TestReadmeDosComandosInstalacion(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	md := string(raw)
	fresh := "irm https://raw.githubusercontent.com/carlos-burelo/rkt/main/scripts/install.ps1 | iex"
	nuke := "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/carlos-burelo/rkt/main/scripts/install.ps1))) -Destructivo"
	for _, want := range []string{fresh, nuke} {
		found := 0
		for _, ln := range strings.Split(md, "\n") {
			if strings.TrimSpace(ln) == want {
				found++
			}
		}
		if found != 1 {
			t.Errorf("README: comando en 1 línea x%d (quiere x1): %s", found, want)
		}
	}
	if !strings.Contains(md, "Destructiva") || !strings.Contains(md, "borra lo anterior") {
		t.Error("README: la destructiva debe avisar qué borra")
	}
	pwsh := needPwsh(t)
	for _, one := range []string{fresh, nuke} {
		if errs := pwshParseErrs(t, pwsh, one, false); len(errs) > 0 {
			t.Errorf("comando README no parsea:\n%s", strings.Join(errs, "\n"))
		}
	}
}
