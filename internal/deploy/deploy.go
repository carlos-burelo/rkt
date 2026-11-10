// Package deploy: publica apps desde git.
// pull (clone) + build + canario + corte + rollback.
//
// Layout en disco (app.Dir = slot activo, termina en .a o .b):
//
//	<name>.a/        slot A (código)
//	<name>.b/        slot B (código)
//
// Zero downtime (blue-green): el build ocurre en el slot inactivo, se
// arranca un canario en un puerto efímero, se espera a que responda HTTP
// y solo entonces se corta la ruta (Live.Swap) y se adopta el canario,
// matando la instancia vieja. La app nunca deja de servir: el corte es
// atómico a nivel de config y el canario ya está sano antes.
//
// Trade-off: el build escribe sobre el slot inactivo, así que un build
// fallido deja ese slot sin la versión anterior (la app activa sigue viva;
// no hay nada que deshacer porque no se desplegó). El rollback manual o
// automático tras un deploy exitoso sí tiene la versión anterior intacta.
//
// El token (PAT contents:read) NUNCA va en YAML: se lee de env y viaja solo
// en header HTTP de git (-c), jamás se persiste en .git/config.
package deploy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/platform"
	"rkt-proxy/internal/supervisor"
)

// maxDeploy acota clone+build+corte: un repo colgado no bloquea la cola.
const maxDeploy = 30 * time.Minute

// httpWait tope del probe HTTP del canario (debe responder antes del corte).
const httpWait = 15 * time.Second

// Controller lo implementa quien posee los procesos (el servicio).
type Controller interface {
	// SwapAndRestart ejecuta swap con la app muerta y arranca al instante.
	// (build local y rutas sin blue-green).
	SwapAndRestart(name string, swap func() error) error
	// Snapshot estado actual de apps.
	Snapshot() []supervisor.Status
	// StartCanary arranca la app nueva en dir con port (proceso crudo), sin
	// tocar la instancia actual. El caller posee el *exec.Cmd hasta Cutover.
	StartCanary(name, dir, port string, out io.Writer) (*exec.Cmd, error)
	// Cutover persiste (Dir, PORT, ruta→port) y adopta el canario matando
	// la instancia vieja. El corte de tráfico es atómico a nivel de config.
	Cutover(name string, cmd *exec.Cmd, dir, port string) error
}

// Run despliega sin cancelación (CLI/tests). El servicio usa RunContext.
func Run(app config.App, token string, ctl Controller, out io.Writer, log *slog.Logger) (string, error) {
	return RunContext(context.Background(), app, token, ctl, out, log)
}

// RunContext despliega y devuelve la rev publicada.
// Zero downtime: construye en el slot inactivo, arranca un canario en un
// puerto efímero, espera a que responda HTTP y solo entonces corta la ruta
// y adopta el canario (matando la instancia vieja). Sin puerto HTTP la app
// se corta igual: el canario se valida por proceso estable.
func RunContext(ctx context.Context, app config.App, token string, ctl Controller, out io.Writer, log *slog.Logger) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, maxDeploy)
	defer cancel()
	d := app.Deploy
	live := app.Dir
	inactive := OtherSlot(live)

	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git no instalado en PATH")
	}
	if NeedsAuth(d.Repo) && token == "" {
		return "", fmt.Errorf("falta token en env %s", d.TokenEnv)
	}

	_ = os.RemoveAll(inactive)
	if err := gitClone(ctx, d, token, inactive, log); err != nil {
		return "", err
	}
	rev := gitRev(ctx, inactive)
	buildEnv := map[string]string{}
	for k, v := range app.Env {
		buildEnv[k] = v
	}
	buildEnv["CI"] = "1"
	if _, ok := buildEnv["NODE_ENV"]; !ok {
		buildEnv["NODE_ENV"] = "production"
	}
	if err := RunStepsContext(ctx, d.Build, inactive, buildEnv, out); err != nil {
		_ = os.RemoveAll(inactive)
		return "", err
	}
	if err := preservePersist(app, live, inactive); err != nil {
		_ = os.RemoveAll(inactive)
		return "", err
	}
	if err := RunStepsContext(ctx, d.Release, inactive, buildEnv, out); err != nil {
		_ = os.RemoveAll(inactive)
		return "", err
	}

	port, err := freeEphemeralPort()
	if err != nil {
		_ = os.RemoveAll(inactive)
		return "", err
	}
	canario, err := ctl.StartCanary(app.Name, inactive, port, out)
	if err != nil {
		_ = os.RemoveAll(inactive)
		return "", err
	}
	if err := waitHTTP(port, app.HealthPath, httpWait); err != nil {
		_ = canario.Process.Kill()
		_ = os.RemoveAll(inactive)
		fmt.Fprintf(out, "\n✗ canario no responde: %v\n", err)
		return "", fmt.Errorf("canario no responde: %w", err)
	}
	// Corte atómico: persiste Dir/PORT/ruta y adopta el canario. La ruta
	// cambia en el próximo request (Live.Swap); el canario ya está sano.
	if err := ctl.Cutover(app.Name, canario, inactive, port); err != nil {
		_ = canario.Process.Kill()
		_ = os.RemoveAll(inactive)
		fmt.Fprintf(out, "\n✗ corte fallido: %v\n", err)
		return "", err
	}
	log.Info("deploy ok (zero downtime)", "app", app.Name, "rev", rev, "port", port)
	return rev, nil
}

// OtherSlot devuelve el slot inactivo para un dir activo terminado en .a/.b.
// Un dir sin sufijo (app antigua) devuelve .b: el primer blue-green migra.
func OtherSlot(dir string) string {
	if strings.HasSuffix(dir, ".a") {
		return strings.TrimSuffix(dir, ".a") + ".b"
	}
	if strings.HasSuffix(dir, ".b") {
		return strings.TrimSuffix(dir, ".b") + ".a"
	}
	return dir + ".b"
}

// freeEphemeralPort pide un puerto libre al SO (efímero). El puerto real de
// la app es el que guarda la ruta; no se reserva del pool.
func freeEphemeralPort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	return port, err
}

// waitHTTP espera respuesta HTTP en 127.0.0.1:port (cualquier status < 500
// cuenta: 404 es una app viva, 502 no).
func waitHTTP(port, path string, timeout time.Duration) error {
	if path == "" {
		path = "/"
	}
	url := "http://127.0.0.1:" + port + path
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var last error
	for {
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("app no responde en %s: %v (el start debe escuchar en $PORT)", url, last)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// WaitAppHTTP espera respuesta HTTP del puerto de la app (cualquier status
// < 500 cuenta: 404 es una app viva, 502 no). Sin puerto no hay probe.
func WaitAppHTTP(app config.App, timeout time.Duration) error {
	port := app.Env["PORT"]
	if port == "" {
		return nil
	}
	return waitHTTP(port, app.HealthPath, timeout)
}

// Rollback vuelve al slot inactivo (la versión anterior) con blue-green:
// arranca un canario desde ese slot en un puerto efímero, valida y corta.
// Para `rollback <app>` manual (el automático vive en Run).
func Rollback(app config.App, ctl Controller, log *slog.Logger) error {
	live := app.Dir
	prev := OtherSlot(live)
	if !dirExists(prev) {
		return fmt.Errorf("sin versión previa en %s", prev)
	}
	port, err := freeEphemeralPort()
	if err != nil {
		return err
	}
	canario, err := ctl.StartCanary(app.Name, prev, port, io.Discard)
	if err != nil {
		return err
	}
	if err := waitHTTP(port, app.HealthPath, httpWait); err != nil {
		_ = canario.Process.Kill()
		return fmt.Errorf("rollback canario no responde: %w", err)
	}
	if err := ctl.Cutover(app.Name, canario, prev, port); err != nil {
		_ = canario.Process.Kill()
		return fmt.Errorf("rollback corte: %w", err)
	}
	log.Info("rollback ok (blue-green)", "app", app.Name)
	return nil
}

func NeedsAuth(repo string) bool {
	return strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://")
}

// isSHA acepta 7–40 hex: los deployments guardan el rev corto y retry
// debe re-clonar el commit, no intentar `--branch <sha>`.
func isSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// gitCmd construye git con credenciales en el entorno del hijo, nunca en
// argv: la lista de procesos no debe mostrar el token.
func gitCmd(ctx context.Context, token string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if token != "" {
		cred := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+cred,
		)
	}
	cmd.Env = env
	return cmd
}

func gitClone(ctx context.Context, d *config.Deploy, token, staging string, log *slog.Logger) error {
	log.Info("clonando", "repo", d.Repo, "ref", d.Ref)
	if isSHA(d.Ref) {
		if out, err := gitCmd(ctx, token, "clone", d.Repo, staging).CombinedOutput(); err != nil {
			return fmt.Errorf("clone: %v: %s", err, OneLine(out))
		}
		if out, err := gitCmd(ctx, token, "-C", staging, "checkout", d.Ref).CombinedOutput(); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("checkout %s: %v: %s", d.Ref, err, OneLine(out))
		}
		return nil
	}
	if out, err := gitCmd(ctx, token, "clone", "--depth", "1", "--branch", d.Ref, d.Repo, staging).CombinedOutput(); err != nil {
		return fmt.Errorf("clone: %v: %s", err, OneLine(out))
	}
	return nil
}

func gitRev(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(out))
}

// HeadRev sha completo del repo en dir ("" si no es repo git).
func HeadRev(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RunSteps ejecuta pasos de build sin cancelación (CLI/tests).
func RunSteps(steps []string, dir string, env map[string]string, out io.Writer) error {
	return RunStepsContext(context.Background(), steps, dir, env, out)
}

// RunStepsContext ejecuta pasos de build en orden en dir.
func RunStepsContext(ctx context.Context, steps []string, dir string, env map[string]string, out io.Writer) error {
	for _, step := range steps {
		if err := runStepContext(ctx, step, dir, env, out); err != nil {
			return fmt.Errorf("build %q: %w", step, err)
		}
	}
	return nil
}

// runStepContext ejecuta un paso de build en el shell de la plataforma.
func runStepContext(ctx context.Context, step, dir string, env map[string]string, out io.Writer) error {
	cmd := platform.ShellContext(ctx, step)
	cmd.Dir = dir
	cmd.Env = dotenv.EnvironListApp(env)
	cmd.Stdout = out
	cmd.Stderr = out
	// marcador estable para checklist CI en dashboard (SPEC 12.2)
	fmt.Fprintf(out, "▶ step: %s\n", step)
	start := time.Now()
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "✘ step failed: %s (%s)\n", step, time.Since(start).Round(time.Millisecond))
		return err
	}
	fmt.Fprintf(out, "✔ step ok: %s (%s)\n", step, time.Since(start).Round(time.Millisecond))
	return nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// preservePersist copia las rutas persistentes del slot activo al slot
// inactivo recién construido, antes del corte. Sin esto, data/uploads/sqlite
// se pierden en cada deploy porque el slot reemplaza el árbol completo.
func preservePersist(app config.App, live, staging string) error {
	for _, rel := range app.Persist {
		clean := filepath.Clean(filepath.FromSlash(rel))
		if clean == "." || clean == ".." || filepath.IsAbs(clean) ||
			strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("persist inválido %q", rel)
		}
		src := filepath.Join(live, clean)
		if _, err := os.Stat(src); err != nil {
			continue // aún no existe: primer deploy
		}
		dst := filepath.Join(staging, clean)
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("persist %s: %w", rel, err)
		}
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("persist %s: %w", rel, err)
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
	if err := os.MkdirAll(dst, st.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// FileChange cambio entre dos revisiones del repo.
type FileChange struct {
	Status string `json:"status"`
	From   string `json:"from,omitempty"`
	Path   string `json:"path"`
}

// DiffFiles lista archivos cambiados entre dos revisiones del repo de la app.
// No usa shell; from/to se validan para no interpretarse como flags.
func DiffFiles(dir, from, to string) ([]FileChange, error) {
	if !dirExists(dir) {
		return nil, fmt.Errorf("directorio inexistente: %s", dir)
	}
	for _, rev := range []string{from, to} {
		if rev == "" || strings.HasPrefix(rev, "-") || strings.ContainsAny(rev, " \t\r\n") {
			return nil, fmt.Errorf("revisión inválida: %q", rev)
		}
	}
	out, err := exec.Command("git", "-C", dir, "diff", "--name-status", "--find-renames", from, to, "--").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	changes := []FileChange{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		c := FileChange{Status: f[0]}
		if len(f) >= 3 {
			c.From, c.Path = f[1], f[2]
		} else {
			c.Path = f[1]
		}
		changes = append(changes, c)
		if len(changes) >= 500 {
			break
		}
	}
	return changes, nil
}

// OneLine primera línea, máx 300, para errores de deploy/servicio.
// Canónica; MCP la reutiliza (antes duplicada con límite 200).
func OneLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
