// Package supervisor: mini-PaaS. Levanta los procesos de apps:,
// les captura stdout/stderr a logs/apps/<name>.log y los reinicia
// con delay si mueren. Con tope anti-crashloop: 5 muertes en 60s
// dejan la app parada (failed) hasta el próximo restart del servicio.
package supervisor

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/dotenv"
	"rkt-proxy/internal/logx"
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
	logFile  *logx.RotatingFile
	done     chan struct{}
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
	// adopt: proceso ya arrancado (canario) que el loop debe tomar en la
	// siguiente vuelta en vez de start(). Lo setea SwapLive.
	adopt    *exec.Cmd
	lastExit string
	crashes  []time.Time
	stopCh   chan struct{}
	restartC chan struct{}
	resumeC  chan struct{}
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
		if a.External() {
			continue
		}
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
	eff := make([]config.App, 0, len(specs))
	for _, s := range specs {
		if s.External() || s.Disabled {
			continue
		}
		eff = append(eff, m.effective(s))
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
	var start, stop []*app
	for _, s := range eff {
		old, ok := prev[s.Name]
		if ok && specsEqual(old.cfg, s) {
			next = append(next, old)
			continue
		}
		if ok {
			stop = append(stop, old)
		}
		na := m.makeApp(s)
		na.onEvent = m.OnEvent
		next = append(next, na)
		start = append(start, na)
	}
	for name, a := range prev {
		if _, keep := want[name]; !keep {
			stop = append(stop, a)
		}
	}
	m.apps = next
	m.mu.Unlock()

	// Fuera del lock: matar procesos no debe bloquear snapshot/restart.
	for _, a := range stop {
		a.stop()
	}
	for _, a := range start {
		a.log.Info("spec aplicada")
		go a.loop()
	}
}

// snapshotApps copia la lista bajo lock. m.apps se reemplaza en Apply:
// iterar el slice vivo sin lock era una carrera real.
func (m *Manager) snapshotApps() []*app {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*app, len(m.apps))
	copy(out, m.apps)
	return out
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
	for _, a := range m.snapshotApps() {
		if a.cfg.Disabled {
			continue
		}
		a.onEvent = m.OnEvent
		go a.loop()
	}
}

// Stop mata todo sin reiniciar (no espera recolección).
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopping = true
	m.mu.Unlock()
	for _, a := range m.snapshotApps() {
		a.stop()
	}
}

// Restart reinicia una app ya: resetea contadores crash, mata la actual
// y salta el delay. Revive failed. False si no existe o se está parando.
func (m *Manager) Restart(name string) bool {
	m.mu.Lock()
	stopping := m.stopping
	apps := make([]*app, len(m.apps))
	copy(apps, m.apps)
	m.mu.Unlock()
	if stopping {
		return false
	}
	for _, a := range apps {
		if a.cfg.Name != name {
			continue
		}
		if a.cfg.Disabled {
			return false
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
			// Sin proceso vivo no hay muerte "pedida" que marcar: el flag
			// quedaría true y la primera muerte natural del proceso nuevo
			// se clasificaría intentional (sin crash, sin backoff).
			a.mu.Lock()
			a.intentional = false
			a.mu.Unlock()
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
		// Sin proceso vivo: el flag intentional quedaría stale y la primera
		// muerte natural del proceso nuevo no contaría como crash.
		a.mu.Lock()
		a.intentional = false
		a.mu.Unlock()
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
	for _, a := range m.snapshotApps() {
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
		a.mu.Lock()
		adopt := a.adopt
		a.adopt = nil
		a.mu.Unlock()
		if adopt != nil {
			// Proceso canario ya arrancado y sano: adoptarlo en vez de start.
			if !a.attach(job, adopt) {
				continue
			}
		} else if !a.start(job) {
			return // parado a propósito o fallo fatal de arranque
		}
		startedAt := a.startedAt()
		err := a.cmd.Wait()
		a.mu.Lock()
		done, lf := a.done, a.logFile
		a.done, a.logFile = nil, nil
		a.mu.Unlock()
		if done != nil {
			close(done)
		}
		if lf != nil {
			_ = lf.Close()
		}
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

		if fast && crashes >= maxCrashes {
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
		timer := time.NewTimer(delay)
		select {
		case <-a.stopCh:
			timer.Stop()
			return
		case <-a.restartC:
			// restart manual: sin espera
			timer.Stop()
		case <-a.resumeC:
			// resume tardío de SwapAndRestart: sin espera
			timer.Stop()
		case <-timer.C:
		}
	}
}

// backoffFor duplica por crash rápido seguido, topado a maxBackoff.
// base <= 0 (no llega por config validada, sí por tests) = sin espera.
func backoffFor(base time.Duration, consec int) time.Duration {
	if base <= 0 {
		return 0
	}
	d := base
	for i := 0; i < consec && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// attach registra un proceso ya arrancado (canario) como la instancia
// supervisada: asigna el job y fija estado para que Wait() lo rastree.
// El stdout/stderr ya lo apunta quien arrancó el canario; el reinicio
// posterior sí abre el log de la app (start()).
func (a *app) attach(job *jobCtx, cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	a.mu.Lock()
	a.cmd = cmd
	a.done = make(chan struct{})
	a.running = true
	a.pid = cmd.Process.Pid
	a.started = time.Now()
	a.mu.Unlock()
	if job != nil {
		if err := job.Assign(uint32(cmd.Process.Pid)); err != nil {
			a.log.Warn("sin job assign (canario)", "err", err)
		}
	}
	a.log.Info("app arriba (canario adoptado)", "pid", cmd.Process.Pid)
	return true
}

// SwapLive reemplaza la instancia actual por un proceso ya arrancado y
// sano (el canario), matando la vieja. El corte de tráfico ya ocurrió en
// la ruta; aquí solo se transfiere la supervisión sin arrancar de cero.
// dir/port (no vacíos) actualizan la spec para que los reinicios
// posteriores usen el slot y puerto nuevos.
func (m *Manager) SwapLive(name string, cmd *exec.Cmd, dir, port string) error {
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
	m.mu.Unlock()

	a.mu.Lock()
	a.adopt = cmd
	a.failed = false
	a.crashes = nil
	a.consec = 0
	a.intentional = true
	if dir != "" {
		a.cfg.Dir = dir
	}
	if port != "" {
		if a.cfg.Env == nil {
			a.cfg.Env = map[string]string{}
		}
		a.cfg.Env["PORT"] = port
	}
	alive := a.alive
	a.mu.Unlock()

	a.killCurrent()
	if !alive {
		go a.loop()
	}
	return nil
}

// StartCanary arranca la app en dir con PORT=port (proceso crudo, sin job
// object), para el blue-green: la instancia actual sigue viva y sirviendo.
// El deploy posee el proceso hasta Cutover (SwapLive) o lo mata si el corte
// no ocurre. out captura stdout/stderr del canario.
func (m *Manager) StartCanary(name, dir, port string, out io.Writer) (*exec.Cmd, error) {
	m.mu.Lock()
	var a *app
	for _, x := range m.apps {
		if x.cfg.Name == name {
			a = x
		}
	}
	if a == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("app %q inexistente", name)
	}
	m.mu.Unlock()

	env := map[string]string{}
	for k, v := range a.cfg.Env {
		env[k] = v
	}
	env["PORT"] = port
	cmd := exec.Command(a.cfg.Command, dotenv.ExpandArgs(a.cfg.Args, env)...)
	configureProc(cmd)
	cmd.Dir = dir
	cmd.Env = dotenv.EnvironListApp(env)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("canario no arrancó: %w", err)
	}
	return cmd, nil
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

	// Los args pueden referenciar el entorno ($PORT): se expanden con el
	// env efectivo para que runtimes sin lectura de env (php -S, puma,
	// dotnet --urls, deno serve) reciban el puerto asignado.
	cmd := exec.Command(a.cfg.Command, dotenv.ExpandArgs(a.cfg.Args, a.cfg.Env)...)
	configureProc(cmd)
	cmd.Dir = a.cfg.Dir
	cmd.Env = dotenv.EnvironListApp(a.cfg.Env)
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		a.log.Error("no arrancó", "err", err)
		a.mu.Lock()
		a.failed = true
		a.lastExit = err.Error()
		a.mu.Unlock()
		return false
	}
	a.mu.Lock()
	a.cmd = cmd
	a.logFile = f
	a.done = make(chan struct{})
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

// openAppLog abre con rotación en caliente (10MB, un .1).
func openAppLog(path string) (*logx.RotatingFile, error) {
	return logx.NewRotating(path, 10<<20)
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

// stopGrace ventana para que la app cierre ordenadamente antes del kill.
var stopGrace = 5 * time.Second

// killCurrent pide cierre ordenado y cae al kill duro si no termina.
// El Wait único lo hace el loop; aquí solo se espera su señal.
func (a *app) killCurrent() {
	a.mu.Lock()
	cmd := a.cmd
	job := a.job
	done := a.done
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		if job != nil {
			job.Kill()
		}
		return
	}
	pid := cmd.Process.Pid
	if err := requestStop(pid); err == nil && done != nil {
		select {
		case <-done:
			return
		case <-time.After(stopGrace):
		}
	}
	// Job Object: mata npm + node/hijos. Sin esto, el hijo conserva el
	// puerto y el cwd, el swap no puede renombrar y la app nueva muere.
	if job != nil {
		job.Kill()
	}
	_ = cmd.Process.Kill()
	if done != nil {
		<-done
	}
}

func exitString(err error) string {
	if err == nil {
		return "exit 0"
	}
	return fmt.Sprintf("%v", err)
}
