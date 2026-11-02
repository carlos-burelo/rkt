// Package supervisor: mini-PaaS. Levanta los procesos de apps:,
// les captura stdout/stderr a logs/apps/<name>.log y los reinicia
// con delay si mueren. Con tope anti-crashloop: 5 muertes en 60s
// dejan la app parada (failed) hasta el próximo restart del servicio.
package supervisor

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"rkt-proxy/internal/config"
)

// crashWindow / maxCrashes: tope anti-crashloop.
// maxBackoff: tope del backoff exponencial.
const (
	crashWindow = time.Minute
	maxCrashes  = 5
	maxBackoff  = 5 * time.Minute
)

// Status foto de una app para healthz.
type Status struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Pid      int    `json:"pid,omitempty"`
	Restarts int    `json:"restarts"`
	Failed   bool   `json:"failed,omitempty"`
	LastExit string `json:"last_exit,omitempty"`
}

type app struct {
	cfg      config.App
	log      *slog.Logger
	logPath  string
	mu       sync.Mutex
	cmd      *exec.Cmd
	running  bool
	alive    bool
	pid      int
	started  time.Time
	restarts int
	consec   int
	failed   bool
	lastExit string
	crashes  []time.Time
	stopCh   chan struct{}
	restartC chan struct{}
	stopped  bool
}

func (a *app) startedAt() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.started
}

// Manager supervisa N apps.
type Manager struct {
	mu       sync.Mutex
	apps     []*app
	stopping bool
}

// New valida rutas de log pero no arranca nada. logsDir/<name>.log.
func New(apps []config.App, logsDir string, log *slog.Logger) *Manager {
	m := &Manager{}
	for _, a := range apps {
		m.apps = append(m.apps, &app{
			cfg:      a,
			log:      log.With("app", a.Name),
			logPath:  filepath.Join(logsDir, a.Name+".log"),
			stopCh:   make(chan struct{}),
			restartC: make(chan struct{}, 1),
		})
	}
	return m
}

// Start lanza todas en goroutines. No bloquea.
func (m *Manager) Start() {
	for _, a := range m.apps {
		go a.loop()
	}
}

// Stop mata todo sin reiniciar (no espera recolección).
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopping = true
	m.mu.Unlock()
	for _, a := range m.apps {
		a.stop()
	}
}

// Restart reinicia una app ya: resetea contadores crash, mata la actual
// y salta el delay. Revive failed. False si no existe o se está parando.
func (m *Manager) Restart(name string) bool {
	m.mu.Lock()
	if m.stopping {
		m.mu.Unlock()
		return false
	}
	m.mu.Unlock()
	for _, a := range m.apps {
		if a.cfg.Name != name {
			continue
		}
		a.mu.Lock()
		a.failed = false
		a.crashes = nil
		a.consec = 0
		alive := a.alive
		a.mu.Unlock()
		a.log.Info("restart manual")
		select {
		case a.restartC <- struct{}{}:
		default:
		}
		if alive {
			a.killCurrent()
		} else {
			go a.loop()
		}
		return true
	}
	return false
}

// Snapshot estado actual para healthz.
func (m *Manager) Snapshot() []Status {
	out := []Status{}
	for _, a := range m.apps {
		a.mu.Lock()
		out = append(out, Status{
			Name: a.cfg.Name, Running: a.running, Pid: a.pid,
			Restarts: a.restarts, Failed: a.failed, LastExit: a.lastExit,
		})
		a.mu.Unlock()
	}
	return out
}

// LogPath ruta del log de una app (para `logs -app`).
func (m *Manager) LogPath(name string) (string, bool) {
	for _, a := range m.apps {
		if a.cfg.Name == name {
			return a.logPath, true
		}
	}
	return "", false
}

func (a *app) loop() {
	a.mu.Lock()
	a.alive = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.alive = false
		a.mu.Unlock()
	}()
	job, err := newJob(a.cfg.MemoryMB)
	if err != nil {
		a.log.Warn("sin job object (sin límites ni kill con padre)", "err", err)
		job = nil
	} else if a.cfg.MemoryMB > 0 {
		a.log.Info("límite RAM", "mb", a.cfg.MemoryMB)
	}
	if job != nil {
		defer job.Close()
	}
	for {
		if !a.start(job) {
			return // parado a propósito o fallo fatal de arranque
		}
		startedAt := a.startedAt()
		err := a.cmd.Wait()
		uptime := time.Since(startedAt)
		fast := uptime < a.cfg.MinUp()

		a.mu.Lock()
		a.running = false
		a.pid = 0
		a.lastExit = exitString(err)
		prev := a.consec
		if fast {
			now := time.Now()
			kept := a.crashes[:0]
			for _, t := range a.crashes {
				if now.Sub(t) < crashWindow {
					kept = append(kept, t)
				}
			}
			a.crashes = append(kept, now)
			a.consec++
		} else {
			a.consec = 0 // vivió lo suficiente: no es crash
		}
		crashes := len(a.crashes)
		a.mu.Unlock()

		select {
		case <-a.stopCh:
			return
		default:
		}

		if fast && crashes > maxCrashes {
			a.mu.Lock()
			a.failed = true
			a.mu.Unlock()
			a.log.Error("crashloop, ya no reintento", "muertes", crashes)
			return
		}
		delay := backoffFor(a.cfg.Delay(), prev)
		a.mu.Lock()
		a.restarts++
		n := a.restarts
		a.mu.Unlock()
		a.log.Warn("app murió, reinicio", "intento", n, "en", delay.String(), "uptime", uptime.String())
		select {
		case <-a.stopCh:
			return
		case <-a.restartC:
			// restart manual: sin espera
		case <-time.After(delay):
		}
	}
}

// backoffFor duplica por crash rápido seguido, topado a maxBackoff.
func backoffFor(base time.Duration, consec int) time.Duration {
	d := base
	for i := 0; i < consec && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff || d <= 0 {
		return maxBackoff
	}
	return d
}

// start lanza el proceso una vez y lo mete al job. False = no reintentar.
func (a *app) start(job *jobCtx) bool {
	select {
	case <-a.stopCh:
		return false
	default:
	}
	if err := os.MkdirAll(filepath.Dir(a.logPath), 0o755); err != nil {
		a.log.Error("sin log dir", "err", err)
		a.mu.Lock()
		a.failed = true
		a.lastExit = err.Error()
		a.mu.Unlock()
		return false
	}
	f, err := openAppLog(a.logPath)
	if err != nil {
		a.log.Error("sin log app", "err", err)
		a.mu.Lock()
		a.failed = true
		a.lastExit = err.Error()
		a.mu.Unlock()
		return false
	}
	defer f.Close()

	cmd := exec.Command(a.cfg.Command, a.cfg.Args...)
	configureProc(cmd)
	cmd.Dir = a.cfg.Dir
	cmd.Env = environWith(a.cfg.Env)
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		a.log.Error("no arrancó", "err", err)
		a.mu.Lock()
		a.failed = true
		a.lastExit = err.Error()
		a.mu.Unlock()
		return false
	}
	a.mu.Lock()
	a.cmd = cmd
	a.running = true
	a.pid = cmd.Process.Pid
	a.started = time.Now()
	a.mu.Unlock()
	if job != nil {
		if err := job.Assign(uint32(cmd.Process.Pid)); err != nil {
			a.log.Warn("sin job assign (sin límites ni kill con padre)", "err", err)
		}
	}
	a.log.Info("app arriba", "pid", cmd.Process.Pid)
	return true
}

// openAppLog abre en append, rotando a .1 si pasa 10MB.
func openAppLog(path string) (*os.File, error) {
	if st, err := os.Stat(path); err == nil && st.Size() > 10<<20 {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func (a *app) stop() {
	select {
	case <-a.stopCh:
		return
	default:
		close(a.stopCh)
	}
	a.killCurrent()
}

// killCurrent mata el proceso actual sin marcar stop (el loop reinicia).
func (a *app) killCurrent() {
	a.mu.Lock()
	cmd := a.cmd
	a.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // cosecha: libera handles y evita carrera con loop
	}
}

// environWith mezcla entorno actual + overrides.
func environWith(over map[string]string) []string {
	base := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				base[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	for k, v := range over {
		base[k] = v
	}
	out := make([]string, 0, len(base))
	for k, v := range base {
		out = append(out, k+"="+v)
	}
	return out
}

func exitString(err error) string {
	if err == nil {
		return "exit 0"
	}
	return fmt.Sprintf("%v", err)
}
