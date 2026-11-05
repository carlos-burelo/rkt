// Package deploy: publica apps desde git.
// pull (clone) + build + swap atómico + restart + verificación + rollback.
//
// Layout en disco (live = app.Dir):
//
//	live/            código sirviendo
//	live.prev/       backup del deploy anterior
//	live.staging/    clonado + build en curso (temporal)
//
// El swap lo ejecuta quien posee los procesos (el servicio, vía Controller):
// en Windows el rename se bloquea con la app viva. El token (PAT
// contents:read) NUNCA va en YAML: se lee de env y viaja solo en header
// HTTP de git (-c), jamás se persiste en .git/config.
package deploy

import (
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/platform"
	"rkt-proxy/internal/supervisor"
)

// healthWait tope esperando app sana tras restart.
const healthWait = 45 * time.Second

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

// Run despliega. Devuelve rev publicado. out = salida del build.
func Run(app config.App, token string, ctl Controller, out io.Writer, log *slog.Logger) (string, error) {
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
	if err := gitClone(d, token, staging, log); err != nil {
		return "", err
	}
	rev := gitRev(staging)
	if err := RunSteps(d.Build, staging, app.Env, out); err != nil {
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
	url := "http://127.0.0.1:" + port + "/"
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
			return fmt.Errorf("app %s no responde en %s: %v", app.Name, url, last)
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

// gitArgs clona con credenciales solo en memoria (-c, sin persistir).
func gitArgs(token string, sub ...string) []string {
	args := []string{"-c", "credential.helper="}
	if token != "" {
		cred := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		args = append(args, "-c", "http.extraHeader=AUTHORIZATION: basic "+cred)
	}
	return append(args, sub...)
}

func gitClone(d *config.Deploy, token, staging string, log *slog.Logger) error {
	log.Info("clonando", "repo", d.Repo, "ref", d.Ref)
	if isSHA(d.Ref) {
		if out, err := exec.Command("git", gitArgs(token, "clone", d.Repo, staging)...).CombinedOutput(); err != nil {
			return fmt.Errorf("clone: %v: %s", err, OneLine(out))
		}
		if out, err := exec.Command("git", gitArgs(token, "-C", staging, "checkout", d.Ref)...).CombinedOutput(); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("checkout %s: %v: %s", d.Ref, err, OneLine(out))
		}
		return nil
	}
	args := gitArgs(token, "clone", "--depth", "1", "--branch", d.Ref, d.Repo, staging)
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("clone: %v: %s", err, OneLine(out))
	}
	return nil
}

func gitRev(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
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

// RunSteps ejecuta pasos de build en orden en dir. Único hogar del shell
// y de los marcadores ▶/✔/✘ que el dashboard usa como checklist.
func RunSteps(steps []string, dir string, env map[string]string, out io.Writer) error {
	for _, step := range steps {
		if err := runStep(step, dir, env, out); err != nil {
			return fmt.Errorf("build %q: %w", step, err)
		}
	}
	return nil
}

// runStep ejecuta un paso de build en el shell de la plataforma.
func runStep(step, dir string, env map[string]string, out io.Writer) error {
	cmd := platform.Shell(step)
	cmd.Dir = dir
	cmd.Env = dotenv.EnvironList(env)
	cmd.Stdout = out
	cmd.Stderr = out
	// marcador estable para checklist CI en dashboard (SPEC 12.2)
	fmt.Fprintf(out, "▶ step: %s\n", step)
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(out, "✘ step failed: %s\n", step)
		return err
	}
	fmt.Fprintf(out, "✔ step ok: %s\n", step)
	return nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
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
