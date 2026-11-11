package deploy

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func mustLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestNeedsAuthTabla: solo http/https exigen token. file/ssh/local no.
func TestNeedsAuthTabla(t *testing.T) {
	si := []string{
		"https://github.com/o/r.git",
		"http://git.local/o/r.git",
		"https://x:x@github.com/o/r.git",
	}
	for _, r := range si {
		if !NeedsAuth(r) {
			t.Fatalf("NeedsAuth(%q) = false, quiere true", r)
		}
	}
	no := []string{
		"",
		"git@github.com:o/r.git",
		"ssh://git@github.com/o/r.git",
		"/srv/git/r.git",
		"./relativo/r.git",
		"C:\\repos\\r.git",
		"file:///srv/git/r.git",
	}
	for _, r := range no {
		if NeedsAuth(r) {
			t.Fatalf("NeedsAuth(%q) = true, quiere false", r)
		}
	}
}

// TestIsSHATabla: 7-40 hex; bordes y mayúsculas.
func TestIsSHATabla(t *testing.T) {
	ok := []string{
		"3cb6176", // 7, mínimo
		"ABCDEF1", // mayúsculas valen
		"0123456789abcdef0123456789abcdef01234567", // 40, máximo
		"6360d12",
	}
	for _, s := range ok {
		if !isSHA(s) {
			t.Fatalf("isSHA(%q) = false, quiere true", s)
		}
	}
	bad := []string{
		"", "main", "v1.2", "3cb61", // 5, corto
		"zzzzzzz",               // no hex
		"3cb617!",               // símbolo
		"3cb 176",               // espacio
		strings.Repeat("a", 41), // largo
		"-3cb6176",              // flag (no debe colarse como rev)
	}
	for _, s := range bad {
		if isSHA(s) {
			t.Fatalf("isSHA(%q) = true, quiere false", s)
		}
	}
}

// TestOneLineTabla: primera línea, trim, tope 300 con ….
func TestOneLineTabla(t *testing.T) {
	if got := OneLine(nil); got != "" {
		t.Fatalf("nil = %q, quiere vacío", got)
	}
	if got := OneLine([]byte("  hola\nsegunda")); got != "hola" {
		t.Fatalf("multilínea = %q, quiere primera recortada", got)
	}
	if got := OneLine([]byte("una-sola")); got != "una-sola" {
		t.Fatalf("una línea = %q", got)
	}
	largo := strings.Repeat("x", 400) + "\ncola"
	got := OneLine([]byte(largo))
	if !strings.HasSuffix(got, "…") || len([]rune(strings.TrimSuffix(got, "…"))) != 300 {
		t.Fatalf("tope 300+…: runas=%d sufijo=%q", len([]rune(got)), got[len(got)-3:])
	}
	exacto := strings.Repeat("y", 300)
	if got := OneLine([]byte(exacto)); got != exacto {
		t.Fatal("300 exactos no deben truncarse")
	}
	// 301 sí trunca (300 runas + …).
	if got := OneLine([]byte(strings.Repeat("z", 301))); !strings.HasSuffix(got, "…") {
		t.Fatalf("301 debe truncar con …: %q", got)
	}
}

// TestGitCmdEnv: token en entorno del hijo, nunca en argv; sin token no hay claves.
func TestGitCmdEnv(t *testing.T) {
	con := gitCmd(t.Context(), "tok-123", "clone", "https://h/o/r.git", "dst")
	for _, a := range con.Args {
		if strings.Contains(a, "tok-123") {
			t.Fatalf("token en argv: %v", con.Args)
		}
	}
	var term, key, val bool
	for _, e := range con.Env {
		if e == "GIT_TERMINAL_PROMPT=0" {
			term = true
		}
		if strings.HasPrefix(e, "GIT_CONFIG_COUNT=1") {
			key = true
		}
		if strings.HasPrefix(e, "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic ") {
			val = true
		}
	}
	if !term || !key || !val {
		t.Fatalf("env con token incompleto: term=%v count=%v value=%v", term, key, val)
	}
	sin := gitCmd(t.Context(), "", "clone", "https://h/o/r.git", "dst")
	for _, e := range sin.Env {
		if strings.HasPrefix(e, "GIT_CONFIG_COUNT") || strings.HasPrefix(e, "GIT_CONFIG_KEY_0") || strings.HasPrefix(e, "GIT_CONFIG_VALUE_0") {
			t.Fatalf("sin token no debe haber GIT_CONFIG_*: %q", e)
		}
	}
	found := false
	for _, e := range sin.Env {
		if e == "GIT_TERMINAL_PROMPT=0" {
			found = true
		}
	}
	if !found {
		t.Fatal("sin token debe conservar GIT_TERMINAL_PROMPT=0")
	}
}

// TestGitAusente: sin git en PATH el deploy muere antes de tocar disco/red.
func TestGitAusente(t *testing.T) {
	vacio := t.TempDir()
	t.Setenv("PATH", vacio)
	if _, err := exec.LookPath("git"); err == nil {
		t.Skip("git resoluble aun sin PATH (imagen con git en sistema)")
	}
	live := filepath.Join(t.TempDir(), "live.a")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Run(testApp(live, "https://example.com/o/r.git", nil), "tok", &fakeCtl{}, io.Discard, mustLog())
	if err == nil || !strings.Contains(err.Error(), "git no instalado") {
		t.Fatalf("quiere 'git no instalado', got %v", err)
	}
}

// TestRunHTTPsinToken: http exige token y falla sin tocar red.
func TestRunHTTPsinToken(t *testing.T) {
	gitOK(t)
	live := filepath.Join(t.TempDir(), "live.a")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Run(testApp(live, "http://git.local/o/r.git", nil), "", &fakeCtl{}, io.Discard, mustLog())
	if err == nil || !strings.Contains(err.Error(), "falta token") {
		t.Fatalf("http sin token: quiere 'falta token', got %v", err)
	}
}

// TestRunLocalSinTokenIntentaClone: path local no pide token; falla en clone.
func TestRunLocalSinTokenIntentaClone(t *testing.T) {
	gitOK(t)
	live := filepath.Join(t.TempDir(), "live.a")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Run(testApp(live, filepath.Join(t.TempDir(), "no-existe"), nil), "", &fakeCtl{}, io.Discard, mustLog())
	if err == nil || !strings.Contains(err.Error(), "clone:") {
		t.Fatalf("repo local inexistente: quiere 'clone:', got %v", err)
	}
	if strings.Contains(err.Error(), "falta token") {
		t.Fatalf("repo local no debe pedir token: %v", err)
	}
}

// TestGitCloneBranchMala: --branch inexistente falla y no deja staging.
func TestGitCloneBranchMala(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	staging := filepath.Join(t.TempDir(), "staging")
	d := testApp("", repo, nil).Deploy
	d.Ref = "rama-que-no-existe-xyz"
	err := gitClone(t.Context(), d, "", staging, mustLog())
	if err == nil || !strings.Contains(err.Error(), "clone:") {
		t.Fatalf("branch mala: quiere 'clone:', got %v", err)
	}
}

// TestGitCloneRepoInexistente: path que no existe falla en clone.
func TestGitCloneRepoInexistente(t *testing.T) {
	gitOK(t)
	staging := filepath.Join(t.TempDir(), "staging")
	d := testApp("", filepath.Join(t.TempDir(), "no-existe"), nil).Deploy
	d.Ref = "main"
	if err := gitClone(t.Context(), d, "", staging, mustLog()); err == nil {
		t.Fatal("repo inexistente debe fallar")
	}
}

// TestGitCloneSHACheckoutMalo: clona bien pero el checkout del SHA falla y limpia staging.
func TestGitCloneSHACheckoutMalo(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	base := t.TempDir()
	staging := filepath.Join(base, "staging")
	d := testApp("", repo, nil).Deploy
	d.Ref = "deadbee" // 7 hex: entra por rama SHA pero no existe
	err := gitClone(t.Context(), d, "", staging, mustLog())
	if err == nil || !strings.Contains(err.Error(), "checkout deadbee") {
		t.Fatalf("quiere 'checkout deadbee', got %v", err)
	}
	if _, serr := os.Stat(staging); !os.IsNotExist(serr) {
		t.Fatal("staging debe limpiarse tras checkout fallido")
	}
}

// TestGitRevTabla: repo da short SHA, no-repo da "?".
func TestGitRevTabla(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	if rev := gitRev(t.Context(), repo); rev == "" || rev == "?" || len(rev) < 7 {
		t.Fatalf("gitRev(repo) = %q, quiere short SHA", rev)
	}
	if rev := gitRev(t.Context(), t.TempDir()); rev != "?" {
		t.Fatalf("gitRev(no-repo) = %q, quiere ?", rev)
	}
}

// TestHeadRevTabla: sha completo o "" si no es repo.
func TestHeadRevTabla(t *testing.T) {
	gitOK(t)
	repo, sha := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	if got := HeadRev(repo); got != sha || len(got) != 40 {
		t.Fatalf("HeadRev = %q, quiere %q (40)", got, sha)
	}
	if got := HeadRev(t.TempDir()); got != "" {
		t.Fatalf("HeadRev(no-repo) = %q, quiere vacío", got)
	}
}

// TestFreeEphemeralPort: numérico, en rango y enlazable.
func TestFreeEphemeralPort(t *testing.T) {
	p, err := freeEphemeralPort()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		t.Fatalf("puerto %q no numérico/en rango: %v", p, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+p)
	if err != nil {
		t.Fatalf("puerto %s no enlazable: %v", p, err)
	}
	_ = ln.Close()
}

// waitServe lanza un handler en 127.0.0.1:0 y devuelve el puerto.
func waitServe(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port
}

// TestWaitHTTP404ok: 404 es app viva (no 5xx).
func TestWaitHTTP404ok(t *testing.T) {
	port := waitServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if err := waitHTTP(port, "/", 3*time.Second); err != nil {
		t.Fatalf("404 debe contar como sano: %v", err)
	}
}

// TestWaitHTTPPathVacioUsaRaiz: path "" pide "/".
func TestWaitHTTPPathVacioUsaRaiz(t *testing.T) {
	var gotPath string
	port := waitServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	if err := waitHTTP(port, "", 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/" {
		t.Fatalf("path vacío pidió %q, quiere /", gotPath)
	}
}

// TestWaitHTTPHealthCustom: solo /salud responde 200; "/" da 500.
func TestWaitHTTPHealthCustom(t *testing.T) {
	port := waitServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/salud" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	if err := waitHTTP(port, "/salud", 3*time.Second); err != nil {
		t.Fatalf("/salud debe sanar: %v", err)
	}
	if err := waitHTTP(port, "/", 700*time.Millisecond); err == nil {
		t.Fatal("/ con 500 debe agotar el timeout")
	}
}

// TestWaitHTTP502luego200: canario que calienta (502,502,200) sana sin error.
func TestWaitHTTP502luego200(t *testing.T) {
	n := 0
	port := waitServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	if err := waitHTTP(port, "/", 5*time.Second); err != nil {
		t.Fatalf("502→200 debe sanar: %v", err)
	}
	if n < 3 {
		t.Fatalf("debió reintentar: n=%d", n)
	}
}

// TestWaitHTTP500SiempreAgota: mensaje con URL y causa.
func TestWaitHTTP500SiempreAgota(t *testing.T) {
	port := waitServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	err := waitHTTP(port, "/", 700*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "app no responde") || !strings.Contains(err.Error(), "127.0.0.1:"+port) {
		t.Fatalf("timeout 500: got %v", err)
	}
}

// TestWaitHTTPRefusadaAgota: puerto cerrado agota con la causa conectiva.
func TestWaitHTTPRefusadaAgota(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()
	err = waitHTTP(port, "/", 700*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "app no responde") {
		t.Fatalf("puerto cerrado: got %v", err)
	}
}

// TestWaitAppHTTP: sin PORT nil inmediato; con PORT delega en waitHTTP.
func TestWaitAppHTTP(t *testing.T) {
	if err := WaitAppHTTP(testApp("/x.a", t.TempDir(), nil), time.Second); err != nil {
		t.Fatalf("sin PORT debe ser nil: %v", err)
	}
	port := waitServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	app := testApp("/x.a", t.TempDir(), nil)
	app.Env = map[string]string{"PORT": port}
	if err := WaitAppHTTP(app, 3*time.Second); err != nil {
		t.Fatalf("con PORT sano: %v", err)
	}
	app.Env["PORT"] = "1" // casi seguro cerrado
	if err := WaitAppHTTP(app, 700*time.Millisecond); err == nil {
		t.Fatal("con PORT cerrado debe fallar")
	}
}

// TestRunStepsOrdenYVacio: orden de ejecución y cero pasos = nil.
func TestRunStepsOrdenYVacio(t *testing.T) {
	if err := RunSteps(nil, t.TempDir(), nil, io.Discard); err != nil {
		t.Fatalf("cero pasos debe ser nil: %v", err)
	}
	var out bytes.Buffer
	steps := []string{"echo uno", "echo dos", "echo tres"}
	if err := RunSteps(steps, t.TempDir(), nil, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	i1, i2, i3 := strings.Index(s, "▶ step: echo uno"), strings.Index(s, "▶ step: echo dos"), strings.Index(s, "▶ step: echo tres")
	if i1 < 0 || i2 < 0 || i3 < 0 || !(i1 < i2 && i2 < i3) {
		t.Fatalf("pasos fuera de orden:\n%s", s)
	}
}

// TestRunStepsFallaEnvuelve: el error cita el paso y el tercero no corre.
func TestRunStepsFallaEnvuelve(t *testing.T) {
	var out bytes.Buffer
	err := RunSteps([]string{"echo ok", "exit 1", "echo no-debe-correr"}, t.TempDir(), nil, &out)
	if err == nil || !strings.Contains(err.Error(), `build "exit 1"`) {
		t.Fatalf("quiere build \"exit 1\", got %v", err)
	}
	if strings.Contains(out.String(), "echo no-debe-correr") && strings.Contains(out.String(), "✔ step ok: echo no-debe-correr") {
		t.Fatalf("el tercer paso no debió correr:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "✘ step failed: exit 1") {
		t.Fatalf("falta marcador ✘:\n%s", out.String())
	}
}

// TestRunStepMarkers: ▶ siempre, ✔ si ok, salida del paso visible.
func TestRunStepMarkers(t *testing.T) {
	var out bytes.Buffer
	if err := RunSteps([]string{"echo hola-marcador"}, t.TempDir(), nil, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"▶ step: echo hola-marcador", "✔ step ok: echo hola-marcador", "hola-marcador"} {
		if !strings.Contains(s, want) {
			t.Fatalf("falta %q en:\n%s", want, s)
		}
	}
}

// checkEnvStep devuelve un paso que falla salvo que VAR==want (portable sh/cmd).
func checkEnvStep(varName, want string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`if not "%%%s%%"=="%s" exit 1`, varName, want)
	}
	return fmt.Sprintf(`[ "$%s" = "%s" ]`, varName, want)
}

// TestDeployBuildEnv: CI=1 forzado, NODE_ENV por defecto y vars propias llegan al build.
func TestDeployBuildEnv(t *testing.T) {
	gitOK(t)
	cases := []struct {
		name string
		env  map[string]string
		step func() string
	}{
		{"CI forzado aunque el manifiesto diga 0", map[string]string{"CI": "0"}, func() string { return checkEnvStep("CI", "1") }},
		{"NODE_ENV por defecto production", nil, func() string { return checkEnvStep("NODE_ENV", "production") }},
		{"NODE_ENV del manifiesto se conserva", map[string]string{"NODE_ENV": "development"}, func() string { return checkEnvStep("NODE_ENV", "development") }},
		{"var propia llega al build", map[string]string{"RKT_ECO": "hola123"}, func() string { return checkEnvStep("RKT_ECO", "hola123") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
			live := filepath.Join(t.TempDir(), "live.a")
			if err := os.MkdirAll(live, 0o755); err != nil {
				t.Fatal(err)
			}
			app := testApp(live, repo, []string{c.step()})
			app.Env = c.env
			ctl := &fakeCtl{}
			defer ctl.close()
			if _, err := Run(app, "", ctl, io.Discard, mustLog()); err != nil {
				t.Fatalf("env no llegó al build: %v", err)
			}
		})
	}
}

// TestDeployReleaseFalla: build ok pero release roto = sin corte y slot limpio.
func TestDeployReleaseFalla(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	app := testApp(live, repo, []string{"echo build-ok"})
	app.Deploy.Release = []string{"exit 1"}
	ctl := &fakeCtl{}
	defer ctl.close()
	_, err := Run(app, "", ctl, io.Discard, mustLog())
	if err == nil || !strings.Contains(err.Error(), `build "exit 1"`) {
		t.Fatalf("release roto: got %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el release falla")
	}
	if _, serr := os.Stat(filepath.Join(base, "live.b")); !os.IsNotExist(serr) {
		t.Fatal("slot inactivo debe limpiarse tras release fallido")
	}
}

// TestDeployPersistInvalido: traversal en Persist aborta antes del corte.
func TestDeployPersistInvalido(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	live := filepath.Join(t.TempDir(), "live.a")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	app := testApp(live, repo, nil)
	app.Persist = []string{"../fuera"}
	ctl := &fakeCtl{}
	defer ctl.close()
	_, err := Run(app, "", ctl, io.Discard, mustLog())
	if err == nil || !strings.Contains(err.Error(), "persist") {
		t.Fatalf("quiere error de persist, got %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el persist es inválido")
	}
}

// TestDeploySinSufijoMigraAB: dir sin .a/.b construye en .b (primer blue-green).
func TestDeploySinSufijoMigraAB(t *testing.T) {
	gitOK(t)
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v2\n"})
	base := t.TempDir()
	live := filepath.Join(base, "live") // sin sufijo: app antigua
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	ctl := &fakeCtl{}
	defer ctl.close()
	if _, err := Run(testApp(live, repo, nil), "", ctl, io.Discard, mustLog()); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if want := live + ".b"; ctl.cutoverDir != want {
		t.Fatalf("corte a %q, quiere %q", ctl.cutoverDir, want)
	}
}

// TestDirExistsTabla: dir true; archivo y ausente false.
func TestDirExistsTabla(t *testing.T) {
	dir := t.TempDir()
	if !dirExists(dir) {
		t.Fatal("dir temporal debe existir")
	}
	f := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dirExists(f) {
		t.Fatal("un archivo no es dir")
	}
	if dirExists(filepath.Join(dir, "no-existe")) {
		t.Fatal("ausente debe dar false")
	}
}

// TestCopyTreeRamas: archivo suelto, árbol anidado y error si falta src.
func TestCopyTreeRamas(t *testing.T) {
	base := t.TempDir()
	if err := copyTree(filepath.Join(base, "no-existe"), filepath.Join(base, "dst")); err == nil {
		t.Fatal("src inexistente debe fallar")
	}
	// archivo suelto con contenido.
	srcF := filepath.Join(base, "uno.txt")
	if err := os.WriteFile(srcF, []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	dstF := filepath.Join(base, "sub", "dos.txt")
	if err := copyTree(srcF, dstF); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dstF); string(b) != "hola" {
		t.Fatalf("archivo copiado = %q", b)
	}
	// árbol anidado.
	srcD := filepath.Join(base, "src")
	if err := os.MkdirAll(filepath.Join(srcD, "anid", "ado"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcD, "anid", "ado", "f.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dstD := filepath.Join(base, "dst")
	if err := copyTree(srcD, dstD); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dstD, "anid", "ado", "f.txt")); string(b) != "deep" {
		t.Fatalf("árbol copiado = %q", b)
	}
}

// TestPreservePersistRamas: archivo, dir, skip si falta src, rechazos.
func TestPreservePersistRamas(t *testing.T) {
	// src ausente = skip silencioso (primer deploy).
	root := t.TempDir()
	live := filepath.Join(root, "live")
	staging := filepath.Join(root, "staging")
	os.MkdirAll(live, 0o755)
	os.MkdirAll(staging, 0o755)
	if err := preservePersist(testApp("", "", nil), live, staging); err != nil {
		t.Fatalf("sin persist debe ser nil: %v", err)
	}
	app := testApp("", "", nil)
	app.Persist = []string{"nuevo-dir"}
	if err := preservePersist(app, live, staging); err != nil {
		t.Fatalf("src ausente debe saltarse: %v", err)
	}
	// archivo persistente: el staging trae versión nueva y se pisa con la viva.
	os.WriteFile(filepath.Join(live, "db.sqlite"), []byte("vivo"), 0o644)
	os.WriteFile(filepath.Join(staging, "db.sqlite"), []byte("nuevo"), 0o644)
	app.Persist = []string{"db.sqlite"}
	if err := preservePersist(app, live, staging); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(staging, "db.sqlite")); string(b) != "vivo" {
		t.Fatalf("archivo persist = %q, quiere vivo", b)
	}
	// dir persistente con anidado.
	os.MkdirAll(filepath.Join(live, "data", "sub"), 0o755)
	os.WriteFile(filepath.Join(live, "data", "sub", "f.txt"), []byte("persistente"), 0o644)
	os.MkdirAll(filepath.Join(staging, "data"), 0o755)
	os.WriteFile(filepath.Join(staging, "data", "viejo.txt"), []byte("nuevo"), 0o644)
	app.Persist = []string{"data"}
	if err := preservePersist(app, live, staging); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(staging, "data", "sub", "f.txt")); string(b) != "persistente" {
		t.Fatalf("dir persist = %q", b)
	}
	// rechazos: absoluto real del OS, ".", ".." y traversal.
	absBad := filepath.Join(t.TempDir(), "fuera")
	for _, bad := range []string{absBad, ".", "..", "../fuera", "a/../../fuera"} {
		app.Persist = []string{bad}
		if err := preservePersist(app, live, staging); err == nil {
			t.Fatalf("persist %q debe fallar", bad)
		}
	}
}

// TestDiffFilesRamas: errores de validación, no-repo y casos felices.
func TestDiffFilesRamas(t *testing.T) {
	gitOK(t)
	if _, err := DiffFiles(filepath.Join(t.TempDir(), "no-existe"), "a", "b"); err == nil {
		t.Fatal("dir inexistente debe fallar")
	}
	dir := t.TempDir()
	for _, rev := range [][2]string{{"", "HEAD"}, {"HEAD", ""}, {"-x", "HEAD"}, {"HEAD", "--help"}, {"a b", "HEAD"}} {
		if _, err := DiffFiles(dir, rev[0], rev[1]); err == nil {
			t.Fatalf("rev %q/%q debe rechazarse", rev[0], rev[1])
		}
	}
	// dir existe pero no es repo: git diff falla.
	if _, err := DiffFiles(dir, "HEAD~1", "HEAD"); err == nil {
		t.Fatal("no-repo debe fallar en git diff")
	}
	// sin cambios entre la misma rev: vacío.
	repo, _ := mkRepo(t, map[string]string{"a.txt": "v1\n"})
	sha := HeadRev(repo)
	if sha == "" {
		t.Fatal("sin HEAD")
	}
	got, err := DiffFiles(repo, sha, sha)
	if err != nil || len(got) != 0 {
		t.Fatalf("mismo rev: got=%v err=%v", got, err)
	}
}

// TestDiffFilesRename: R100 con From y Path.
func TestDiffFilesRename(t *testing.T) {
	gitOK(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(dir, "viejo.txt"), []byte("contenido-largo-para-detectar-rename\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "v1", "--no-gpg-sign")
	if err := os.Rename(filepath.Join(dir, "viejo.txt"), filepath.Join(dir, "nuevo.txt")); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "v2", "--no-gpg-sign")
	got, err := DiffFiles(dir, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0].Status, "R") || got[0].From != "viejo.txt" || got[0].Path != "nuevo.txt" {
		t.Fatalf("rename = %+v", got)
	}
}

// TestDiffFilesTope500: 510 añadidos se acotan a 500.
func TestDiffFilesTope500(t *testing.T) {
	gitOK(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir(),
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "v1", "--no-gpg-sign")
	for i := 0; i < 510; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("commit", "-m", "v2", "--no-gpg-sign")
	got, err := DiffFiles(dir, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 500 {
		t.Fatalf("tope: len=%d, quiere 500", len(got))
	}
}

// TestRollbackCanarioFallaArranque: StartCanary roto, sin corte.
func TestRollbackCanarioFallaArranque(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	prev := filepath.Join(base, "live.b")
	os.MkdirAll(live, 0o755)
	os.MkdirAll(prev, 0o755)
	ctl := &fakeCtl{failCanary: true}
	defer ctl.close()
	err := Rollback(testApp(live, "https://example.com/o/r.git", nil), ctl, mustLog())
	if err == nil || strings.Contains(err.Error(), "corte") {
		t.Fatalf("quiere error de canario (no de corte): %v", err)
	}
	if ctl.cutoverDir != "" {
		t.Fatal("sin corte si el canario no arranca")
	}
}

// TestRollbackCorteFalla: canario sano pero Cutover roto.
func TestRollbackCorteFalla(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "live.a")
	prev := filepath.Join(base, "live.b")
	os.MkdirAll(live, 0o755)
	os.MkdirAll(prev, 0o755)
	os.WriteFile(filepath.Join(prev, "old.txt"), []byte("v"), 0o644)
	ctl := &fakeCtl{failCutover: true}
	defer ctl.close()
	err := Rollback(testApp(live, "https://example.com/o/r.git", nil), ctl, mustLog())
	if err == nil || !strings.Contains(err.Error(), "rollback corte") {
		t.Fatalf("quiere 'rollback corte', got %v", err)
	}
}
