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
	"rkt-proxy/internal/dotenv"
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
	onEvent  func(kind, app, msg string)
	logPath  string
	mu       sync.Mutex
	cmd      *exec.Cmd
	job      *jobCtx
	running  bool
	alive    bool
	held     bool
	pid      int
	started  time.Time
	restarts int
	consec   int
	failed   bool
	// intentional: la muerte actual fue pedida (restart/swap), no crash.
	intentional bool
	lastExit    string
	crashes     []time.Time
	stopCh      chan struct{}
	restartC    chan struct{}
	resumeC     chan struct{}
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
	// global se fusiona en el env de cada app al arrancar (la app gana).
	global map[string]string
	// logsDir y base para recrear apps al aplicar config nueva.
	logsDir string
	baseLog *slog.Logger
	// OnEvent, si no nil, recibe died/restarted/failed (auditoría).
	OnEvent   func(kind, app, msg string)
	deploying map[string]bool
}

// New valida rutas de log pero no arranca nada. logsDir/<name>.log.
// global es el entorno compartido (nil = sin globales).
func New(apps []config.App, global map[string]string, logsDir string, log *slog.Logger) *Manager {
	m := &Manager{logsDir: logsDir, baseLog: log, global: global}
	for _, a := range apps {
		m.apps = append(m.apps, m.makeApp(m.effective(a)))
	}
	return m
}

// effective aplica el entorno global sin mutar el manifiesto guardado.
func (m *Manager) effective(a config.App) config.App {
	a.Env = config.MergeEnv(m.global, a.Env)
	return a
}

// makeApp construye el runner de una spec (sin arrancarlo).
func (m *Manager) makeApp(a config.App) *app {
	return &app{
		cfg:      a,
		log:      m.baseLog.With("app", a.Name),
		logPath:  filepath.Join(m.logsDir, a.Name+".log"),
		stopCh:   make(chan struct{}),
		restartC: make(chan struct{}, 1),
		resumeC:  make(chan struct{}, 1),
	}
}

// Apply reconcilia la flota viva con specs nuevas: detiene las retiradas,
// reinicia las que cambiaron y arranca las nuevas. Las iguales no se tocan.
// Es el único camino para aplicar cambios de manifiesto en caliente.
// global entra en el env efectivo: cambiarlo reinicia las apps afectadas.
func (m *Manager) Apply(specs []config.App, global map[string]string) {
	m.mu.Lock()
	if m.stopping {
		m.mu.Unlock()
		return
	}
	m.global = global
	eff := make([]config.App, len(specs))
	for i, s := range specs {
		eff[i] = m.effective(s)
	}
	want := make(map[string]config.App, len(eff))
	for _, s := range eff {
		want[s.Name] = s
	}
	prev := make(map[string]*app, len(m.apps))
	for _, a := range m.apps {
		prev[a.cfg.Name] = a
	}

	next := make([]*app, 0, len(eff))
	var start []*app
	for _, s := range eff {
		old, ok := prev[s.Name]
		if ok && specsEqual(old.cfg, s) {
			next = append(next, old)
			continue
		}
		if ok {
			old.stop()
		}
		na := m.makeApp(s)
		na.onEvent = m.OnEvent
		next = append(next, na)
		start = append(start, na)
	}
	for name, a := range prev {
		if _, keep := want[name]; !keep {
			a.stop()
		}
	}
	m.apps = next
	m.mu.Unlock()

	for _, a := range start {
		a.log.Info("spec aplicada")
		go a.loop()
	}
}

// specsEqual compara lo que afecta al proceso (no metadatos de deploy).
func specsEqual(a, b config.App) bool {
	return a.Command == b.Command &&
		a.Dir == b.Dir &&
		a.Static == b.Static &&
		a.RestartDelay == b.RestartDelay &&
		a.MinUptime == b.MinUptime &&
		a.MemoryMB == b.MemoryMB &&
		a.EnvFile == b.EnvFile &&
		equalStrings(a.Args, b.Args) &&
		equalMap(a.Env, b.Env)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Start lanza todas en goroutines. No bloquea.
func (m *Manager) Start() {
	for _, a := range m.apps {
		a.onEvent = m.OnEvent
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
		a.intentional = true
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

// SwapAndRestart para el loop, ejecuta swap con la app muerta (imprescindible
// en Windows: rename bloqueado por handles abiertos) y arranca al instante.
// Resetea contadores crash. Solo un deploy por app a la vez.
func (m *Manager) SwapAndRestart(name string, swap func() error) error {
	m.mu.Lock()
	if m.stopping {
		m.mu.Unlock()
		return fmt.Errorf("servicio parando")
	}
	var a *app
	for _, x := range m.apps {
		if x.cfg.Name == name {
			a = x
		}
	}
	if a == nil {
		m.mu.Unlock()
		return fmt.Errorf("app %q inexistente", name)
	}
	if m.deploying == nil {
		m.deploying = map[string]bool{}
	}
	if m.deploying[name] {
		m.mu.Unlock()
		return fmt.Errorf("deploy de %s ya en curso", name)
	}
	m.deploying[name] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.deploying, name)
		m.mu.Unlock()
	}()

	a.mu.Lock()
	a.failed = false
	a.crashes = nil
	a.consec = 0
	a.held = true
	a.intentional = true
	alive := a.alive
	a.mu.Unlock()

	a.killCurrent()
	if !a.waitDead(10 * time.Second) {
		a.unhold()
		return fmt.Errorf("la app no murió para el swap")
	}
	err := swap()
	a.mu.Lock()
	a.held = false
	a.mu.Unlock()
	select {
	case a.resumeC <- struct{}{}:
	default:
	}
	select {
	case a.restartC <- struct{}{}:
	default:
	}
	if !alive {
		go a.loop()
	}
	return err
}

// unhold libera un loop en espera (vía de error: nunca dejar colgado).
func (a *app) unhold() {
	a.mu.Lock()
	a.held = false
	a.mu.Unlock()
	select {
	case a.resumeC <- struct{}{}:
	default:
	}
}

// waitDead espera a que el loop marque la app muerta (post-kill).
func (a *app) waitDead(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		running := a.running
		a.mu.Unlock()
		if !running {
			return true
		}
		time.Sleep(50 * time.Millisecond)
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
		a.mu.Lock()
		a.job = job
		a.mu.Unlock()
		defer func() {
			a.mu.Lock()
			a.job = nil
			a.mu.Unlock()
			job.Close()
		}()
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
		intentional := a.intentional
		a.intentional = false
		prev := a.consec
		if !intentional {
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
		}
		crashes := len(a.crashes)
		a.mu.Unlock()

		select {
		case <-a.stopCh:
			return
		default:
		}
		if intentional {
			// restart/swap pedido: ni evento died ni contador de reinicios
			a.mu.Lock()
			held := a.held
			a.mu.Unlock()
			if held {
				select {
				case <-a.stopCh:
					return
				case <-a.resumeC:
				}
			}
			select {
			case <-a.restartC:
			default:
			}
			continue
		}
		if a.onEvent != nil {
			a.onEvent("died", a.cfg.Name, "exit="+a.lastExit+" uptime="+uptime.String())
		}

		a.mu.Lock()
		held := a.held
		a.mu.Unlock()
		if held {
			// deploy en curso: espera al swap, sin contar delay
			select {
			case <-a.stopCh:
				return
			case <-a.resumeC:
			}
		}

		if fast && crashes > maxCrashes {
			a.mu.Lock()
			a.failed = true
			a.mu.Unlock()
			a.log.Error("crashloop, ya no reintento", "muertes", crashes)
			if a.onEvent != nil {
				a.onEvent("failed", a.cfg.Name, fmt.Sprintf("crashloop %d muertes", crashes))
			}
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
		case <-a.resumeC:
			// resume tardío de SwapAndRestart: sin espera
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
	cmd.Env = dotenv.EnvironList(a.cfg.Env)
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
	job := a.job
	a.mu.Unlock()
	// Job Object: mata npm + node/hijos. Sin esto, el hijo conserva el
	// puerto y el cwd, el swap no puede renombrar y la app nueva muere.
	if job != nil {
		job.Kill()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // cosecha: libera handles y evita carrera con loop
	}
}

func exitString(err error) string {
	if err == nil {
		return "exit 0"
	}
	return fmt.Sprintf("%v", err)
}
