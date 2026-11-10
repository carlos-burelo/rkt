// Package deploy: publica apps desde git.
// pull (clone) + build + swap + restart + verificación + rollback.
//
// Layout en disco (live = app.Dir):
//
//	live/            código sirviendo
//	live.prev/       backup del deploy anterior
//	live.staging/    clonado + build en curso (temporal)
//
// El swap son dos renames (live→prev, staging→live); cada rename es
// atómico, la secuencia no. Un crash entre ambos deja live ausente:
// RecoverDir (llamada al arrancar el servicio) restaura prev→live.
//
// El swap lo ejecuta quien posee los procesos (el servicio, vía Controller):
// en Windows el rename se bloquea con la app viva. El token (PAT
// contents:read) NUNCA va en YAML: se lee de env y viaja solo en header
// HTTP de git (-c), jamás se persiste en .git/config.
package deploy

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
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

// healthWait tope esperando app sana tras restart.
const healthWait = 45 * time.Second

// maxDeploy acota clone+build+swap: un repo colgado no bloquea la cola.
const maxDeploy = 30 * time.Minute

// stableFor ventana continua sana antes de dar el deploy por bueno:
// atrapa apps que arrancan y mueren al segundo (puerto ocupado, crashloop).
// var para tests (10ms) sin dormir 5s por deploy.
var stableFor = 5 * time.Second

// httpWait tope del probe HTTP post-restart (app debe responder).
const httpWait = 15 * time.Second

// Controller lo implementa quien posee los procesos (el servicio).
type Controller interface {
	// SwapAndRestart ejecuta swap con la app muerta y arranca al instante.
	SwapAndRestart(name string, swap func() error) error
	// Snapshot estado actual de apps.
	Snapshot() []supervisor.Status
}

// Run despliega sin cancelación (CLI/tests). El servicio usa RunContext.
func Run(app config.App, token string, ctl Controller, out io.Writer, log *slog.Logger) (string, error) {
	return RunContext(context.Background(), app, token, ctl, out, log)
}

// RunContext despliega y devuelve la rev publicada.
func RunContext(ctx context.Context, app config.App, token string, ctl Controller, out io.Writer, log *slog.Logger) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, maxDeploy)
	defer cancel()
	d := app.Deploy
	live := app.Dir
	staging := live + ".staging"
	prev := live + ".prev"

	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git no instalado en PATH")
	}
	if NeedsAuth(d.Repo) && token == "" {
		return "", fmt.Errorf("falta token en env %s", d.TokenEnv)
	}

	_ = os.RemoveAll(staging)
	if err := gitClone(ctx, d, token, staging, log); err != nil {
		return "", err
	}
	rev := gitRev(ctx, staging)
	buildEnv := map[string]string{}
	for k, v := range app.Env {
		buildEnv[k] = v
	}
	buildEnv["CI"] = "1"
	if _, ok := buildEnv["NODE_ENV"]; !ok {
		buildEnv["NODE_ENV"] = "production"
	}
	if err := RunStepsContext(ctx, d.Build, staging, buildEnv, out); err != nil {
		_ = os.RemoveAll(staging)
		return "", err
	}
	if err := preservePersist(app, live, staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", err
	}
	if err := RunStepsContext(ctx, d.Release, staging, buildEnv, out); err != nil {
		_ = os.RemoveAll(staging)
		return "", err
	}

	hadLive := dirExists(live)
	swap := func() error {
		if hadLive {
			_ = os.RemoveAll(prev)
			if err := renameRetry(live, prev); err != nil {
				return fmt.Errorf("backup: %w", err)
			}
		}
		if err := renameRetry(staging, live); err != nil {
			if hadLive {
				_ = renameRetry(prev, live)
			}
			return fmt.Errorf("swap: %w", err)
		}
		return nil
	}
	if err := ctl.SwapAndRestart(app.Name, swap); err != nil {
		_ = os.RemoveAll(staging)
		// Swap parcial: si live desapareció, devolver el backup antes de salir.
		if hadLive && !dirExists(live) && dirExists(prev) {
			_ = ctl.SwapAndRestart(app.Name, func() error {
				return renameRetry(prev, live)
			})
		}
		fmt.Fprintf(out, "\n✗ swap: %v\n", err)
		return "", fmt.Errorf("swap: %w", err)
	}
	if err := WaitAppHealthy(ctl.Snapshot, app.Name, healthWait); err != nil {
		back := func() error {
			if !hadLive {
				_ = os.RemoveAll(live)
				return fmt.Errorf("sin backup previo")
			}
			_ = os.RemoveAll(live)
			return renameRetry(prev, live)
		}
		_ = ctl.SwapAndRestart(app.Name, back)
		fmt.Fprintf(out, "\n✗ app no quedó sana: %v\n↺ rollback aplicado\n", err)
		log.Warn("rollback aplicado", "app", app.Name)
		return "", fmt.Errorf("deploy fallido, rollback aplicado: %w", err)
	}
	// Verificación real: el proceso no basta. Si no responde HTTP, se revierte.
	if err := WaitAppHTTP(app, httpWait); err != nil {
		back := func() error {
			if !hadLive {
				_ = os.RemoveAll(live)
				return fmt.Errorf("sin backup previo")
			}
			_ = os.RemoveAll(live)
			return renameRetry(prev, live)
		}
		_ = ctl.SwapAndRestart(app.Name, back)
		fmt.Fprintf(out, "\n✗ sin respuesta HTTP: %v\n↺ rollback aplicado\n", err)
		log.Warn("rollback por HTTP", "app", app.Name)
		return "", fmt.Errorf("deploy fallido (sin respuesta HTTP), rollback aplicado: %w", err)
	}
	_ = os.RemoveAll(staging)
	log.Info("deploy ok", "app", app.Name, "rev", rev)
	return rev, nil
}

// WaitAppHTTP espera respuesta HTTP del puerto de la app (cualquier status
// < 500 cuenta: 404 es una app viva, 502 no). Sin puerto no hay probe.
func WaitAppHTTP(app config.App, timeout time.Duration) error {
	port := app.Env["PORT"]
	if port == "" {
		return nil
	}
	path := app.HealthPath
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
			return fmt.Errorf("app %s no responde en %s: %v (el start debe escuchar en $PORT; Next: `next start -H 127.0.0.1`, sin -p)", app.Name, url, last)
		}
		time.Sleep(time.Second)
	}
}

// WaitAppHealthy espera app corriendo, no failed y estable durante
// stableFor seguido (poll 0.5s). Una app que arranca y muere al instante
// no cuenta como sana.
func WaitAppHealthy(snap func() []supervisor.Status, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var since time.Time
	for {
		ok := false
		for _, s := range snap() {
			if s.Name == name && s.Running && !s.Failed {
				ok = true
				break
			}
		}
		if ok {
			if since.IsZero() {
				since = time.Now()
			}
			if time.Since(since) >= stableFor {
				return nil
			}
		} else {
			since = time.Time{}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("app %s no estable tras %v", name, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Rollback devuelve live.prev a live con la app parada y reinicia.
// Para `rollback <app>` manual (el automático vive en Run).
func Rollback(app config.App, ctl Controller, log *slog.Logger) error {
	live := app.Dir
	prev := live + ".prev"
	if !dirExists(prev) {
		return fmt.Errorf("sin backup %s", prev)
	}
	back := func() error {
		_ = os.RemoveAll(live)
		return renameRetry(prev, live)
	}
	if err := ctl.SwapAndRestart(app.Name, back); err != nil {
		return fmt.Errorf("rollback swap: %w", err)
	}
	if err := WaitAppHealthy(ctl.Snapshot, app.Name, healthWait); err != nil {
		return fmt.Errorf("rollback sin salud: %w", err)
	}
	log.Info("rollback ok", "app", app.Name)
	return nil
}

// renameRetry reintenta (Windows: AV/indexadores retienen handles).
func renameRetry(old, new string) error {
	var err error
	for i := 0; i < 3; i++ {
		if err = os.Rename(old, new); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
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

// RecoverDir repara el layout de una app tras un crash a mitad del swap
// (dos renames: live→prev y staging→live no son atómicos como secuencia).
// Si live falta, restaura live.prev (o completa staging); limpia un
// staging huérfano. Idempotente: seguro de llamar en cada arranque.
func RecoverDir(live string) error {
	if dirExists(live) {
		if dirExists(live + ".staging") {
			_ = os.RemoveAll(live + ".staging")
		}
		return nil
	}
	prev := live + ".prev"
	if dirExists(prev) {
		if err := renameRetry(prev, live); err != nil {
			return fmt.Errorf("recuperar backup %s: %w", prev, err)
		}
		_ = os.RemoveAll(live + ".staging")
		return nil
	}
	if dirExists(live + ".staging") {
		if err := renameRetry(live+".staging", live); err != nil {
			return fmt.Errorf("recuperar staging %s: %w", live+".staging", err)
		}
		return nil
	}
	return nil // primer deploy: aún no hay directorio
}

// preservePersist copia las rutas persistentes del live anterior al staging
// nuevo antes del swap. Sin esto, data/uploads/sqlite se pierden en cada
// deploy porque staging reemplaza el árbol completo.
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
