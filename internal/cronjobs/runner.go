// Package cronjobs programa y ejecuta tareas cron por app.
// Usa el manifiesto vivo (rutas/env) y persiste cada ejecución en SQLite.
package cronjobs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/platform"
	"rkt-proxy/internal/store"
)

// Runner mantiene el cron vivo y ejecuta tareas.
type Runner struct {
	live    *config.Live
	db      *store.DB
	logsDir string
	log     *slog.Logger

	mu      sync.Mutex
	c       *cron.Cron
	entries map[string]cron.EntryID
	sch     map[string]string
	running map[string]bool
	started bool
}

// New crea el runner (sin arrancar).
func New(live *config.Live, db *store.DB, logsDir string, log *slog.Logger) *Runner {
	return &Runner{
		live: live, db: db, logsDir: logsDir, log: log,
		entries: map[string]cron.EntryID{}, sch: map[string]string{}, running: map[string]bool{},
	}
}

// Start arranca el planificador y aplica la flota.
func (r *Runner) Start() {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.c = cron.New()
	r.c.Start()
	r.started = true
	r.mu.Unlock()
	r.Apply()
}

// Stop detiene el planificador y espera ejecuciones en curso.
func (r *Runner) Stop() {
	r.mu.Lock()
	c := r.c
	r.started = false
	r.mu.Unlock()
	if c != nil {
		ctx := c.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// Apply reconcilia las tareas programadas con la config viva.
func (r *Runner) Apply() {
	cfg := r.live.Get()
	want := map[string]config.CronJob{}
	for _, app := range cfg.Apps {
		if app.Disabled {
			continue
		}
		for _, job := range app.Cron {
			if job.Disabled {
				continue
			}
			want[app.Name+"/"+job.Name] = job
		}
	}
	r.mu.Lock()
	if r.c == nil {
		r.mu.Unlock()
		return
	}
	c := r.c
	for key, id := range r.entries {
		job, ok := want[key]
		if !ok || r.sch[key] != job.Schedule {
			c.Remove(id)
			delete(r.entries, key)
			delete(r.sch, key)
		}
	}
	r.mu.Unlock()

	for key, job := range want {
		if _, err := config.ParseCron(job.Schedule); err != nil {
			r.log.Warn("cron inválido, omitido", "job", key, "err", err)
			continue
		}
		appName, jobName := splitKey(key)
		r.mu.Lock()
		if _, exists := r.entries[key]; exists {
			r.mu.Unlock()
			continue
		}
		id, err := c.AddFunc(job.Schedule, func() { r.execute(appName, jobName, "schedule") })
		if err == nil {
			r.entries[key] = id
			r.sch[key] = job.Schedule
		}
		r.mu.Unlock()
		if err != nil {
			r.log.Warn("cron no programado", "job", key, "err", err)
		}
	}
}

// Running true si la tarea tiene una ejecución en curso.
func (r *Runner) Running(app, job string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[app+"/"+job]
}

// RunNow ejecuta una tarea en segundo plano y devuelve el id de ejecución.
func (r *Runner) RunNow(app, job string) (int64, error) {
	a := cfgfile.FindApp(r.live.Get(), app)
	if a == nil {
		return 0, fmt.Errorf("app %q inexistente", app)
	}
	if a.Disabled {
		return 0, fmt.Errorf("app %q está detenida", app)
	}
	var found *config.CronJob
	for i := range a.Cron {
		if a.Cron[i].Name == job {
			found = &a.Cron[i]
			break
		}
	}
	if found == nil {
		return 0, fmt.Errorf("cron %q inexistente en %s", job, app)
	}
	if found.Disabled {
		return 0, fmt.Errorf("cron %q está desactivado", job)
	}
	id, err := r.db.InsertCronRun(app, job, "")
	if err != nil {
		return 0, err
	}
	go r.run(app, job, id, "manual")
	return id, nil
}

// execute es el callback programado.
func (r *Runner) execute(app, job, trigger string) {
	a := cfgfile.FindApp(r.live.Get(), app)
	if a == nil || a.Disabled {
		return
	}
	id, err := r.db.InsertCronRun(app, job, "")
	if err != nil {
		r.log.Error("cron sin registro", "app", app, "job", job, "err", err)
		return
	}
	r.run(app, job, id, trigger)
}

// run ejecuta y cierra la fila de historial.
func (r *Runner) run(appName, jobName string, runID int64, trigger string) {
	key := appName + "/" + jobName
	r.mu.Lock()
	if r.running[key] {
		r.mu.Unlock()
		_ = r.db.FinishCronRun(runID, "skipped", 0, "ejecución anterior en curso")
		return
	}
	r.running[key] = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, key)
		r.mu.Unlock()
	}()

	cfg := r.live.Get()
	app := cfgfile.FindApp(cfg, appName)
	if app == nil || app.Disabled {
		_ = r.db.FinishCronRun(runID, "skipped", 0, "app inexistente o detenida")
		return
	}
	var job *config.CronJob
	for i := range app.Cron {
		if app.Cron[i].Name == jobName {
			job = &app.Cron[i]
			break
		}
	}
	if job == nil || job.Disabled {
		_ = r.db.FinishCronRun(runID, "skipped", 0, "tarea inexistente o desactivada")
		return
	}
	timeout, err := job.CronTimeout()
	if err != nil {
		_ = r.db.FinishCronRun(runID, "failed", 0, err.Error())
		return
	}
	dir := filepath.Join(r.logsDir, "cron")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_ = r.db.FinishCronRun(runID, "failed", 0, err.Error())
		return
	}
	name := fmt.Sprintf("%s-%s-%s.log", appName, jobName, time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = r.db.FinishCronRun(runID, "failed", 0, err.Error())
		return
	}
	defer out.Close()
	if err := r.db.SetCronRunLog(runID, path); err != nil {
		r.log.Warn("cron log path", "err", err)
	}

	fmt.Fprintf(out, "▶ cron %s/%s (%s) %s\n", appName, jobName, trigger, time.Now().UTC().Format(time.RFC3339))
	env := cfg.RuntimeEnv(app.Env)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	status, exitCode := "ok", 0
	switch job.Kind() {
	case config.CronCommand:
		runCmd := platform.ShellContext(ctx, job.Command)
		runCmd.Dir = app.Dir
		runCmd.Env = dotenv.EnvironListApp(env)
		runCmd.Stdout = out
		runCmd.Stderr = out
		err = runCmd.Run()
		if err != nil {
			status, exitCode = "failed", 1
			if ee, ok := err.(interface{ ExitCode() int }); ok {
				exitCode = ee.ExitCode()
			}
			if ctx.Err() == context.DeadlineExceeded {
				status, exitCode = "timeout", -1
			}
			fmt.Fprintf(out, "✘ %v\n", err)
		}
	case config.CronHTTP:
		target, err := httpTarget(env, job.Path)
		if err != nil {
			status, exitCode = "failed", 1
			fmt.Fprintf(out, "✘ %v\n", err)
			break
		}
		method := job.HTTPMethod()
		fmt.Fprintf(out, "→ %s %s\n", method, target)
		req, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			status, exitCode = "failed", 1
			fmt.Fprintf(out, "✘ %v\n", err)
			break
		}
		resp, err := cronHTTPClient.Do(req)
		if err != nil {
			status, exitCode = "failed", 1
			if ctx.Err() == context.DeadlineExceeded {
				status, exitCode = "timeout", -1
			}
			fmt.Fprintf(out, "✘ %v\n", err)
			break
		}
		fmt.Fprintf(out, "← %s (%s)\n", resp.Status, time.Since(start).Round(time.Millisecond))
		_, readErr := io.Copy(out, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		fmt.Fprintln(out)
		if readErr != nil && ctx.Err() == context.DeadlineExceeded {
			status, exitCode = "timeout", -1
			fmt.Fprintf(out, "✘ %v\n", readErr)
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			status, exitCode = "failed", 1
			fmt.Fprintf(out, "✘ HTTP %s\n", resp.Status)
		}
	default:
		status, exitCode = "failed", 1
		fmt.Fprintf(out, "✘ tipo de tarea inválido %q\n", job.Type)
	}
	if status == "ok" {
		fmt.Fprintln(out, "✔ ok")
	}
	msg := ""
	if status == "timeout" {
		msg = "timeout " + timeout.String()
	}
	if err := r.db.FinishCronRun(runID, status, exitCode, msg); err != nil {
		r.log.Warn("cron historial", "err", err)
	}
	r.log.Info("cron ejecutado", "app", appName, "job", jobName, "status", status, "dur", time.Since(start).String())
}

// cronHTTPClient evita proxies del entorno para hablar con el puerto local.
var cronHTTPClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

func httpTarget(env map[string]string, path string) (string, error) {
	port := strings.TrimSpace(env["PORT"])
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("app sin puerto HTTP válido (PORT=%q)", port)
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("ruta HTTP inválida %q", path)
	}
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(n)) + path, nil
}

func splitKey(key string) (string, string) {
	i := strings.Index(key, "/")
	if i < 0 {
		return key, ""
	}
	return key[:i], key[i+1:]
}
