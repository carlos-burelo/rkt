// Package appusage: consumo real de una app (proceso, disco y tráfico).
// Todo se mide, nada se estima: si el proceso no existe, se dice.
package appusage

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/procstat"
	"rkt-proxy/internal/weblog"
)

// Traffic resumen de la ventana pedida (del access log real).
type Traffic struct {
	WindowS  int     `json:"window_s"`
	Requests int     `json:"requests"`
	Bytes    int64   `json:"bytes"`
	Errors   int     `json:"errors"`
	RPS      float64 `json:"rps"`
	ErrPct   float64 `json:"err_pct"`
}

// Report consumo de una unidad.
type Report struct {
	App       string  `json:"app"`
	Pid       int     `json:"pid"`
	Running   bool    `json:"running"`
	HTTPOK    bool    `json:"http_ok"`
	Restarts  int     `json:"restarts"`
	UptimeS   int64   `json:"uptime_s"`
	CPUPct    float64 `json:"cpu_pct"`
	RSSMB     float64 `json:"rss_mb"`
	PeakRSSMB float64 `json:"peak_rss_mb"`
	VirtualMB float64 `json:"virtual_mb"`
	IOReadMB  float64 `json:"io_read_mb"`
	IOWriteMB float64 `json:"io_write_mb"`
	Handles   uint32  `json:"handles"`
	Threads   uint32  `json:"threads"`
	DiskBytes int64   `json:"disk_bytes"`
	DiskSeen  bool    `json:"disk_partial,omitempty"` // walk cortado por tiempo
	LogBytes  int64   `json:"log_bytes"`
	Traffic   Traffic `json:"traffic"`
}

// Input datos que el llamador ya conoce (estado del supervisor + rutas).
type Input struct {
	App     config.App
	Cfg     *config.Config
	LogsDir string
	Status  health.AppStatus
	Window  time.Duration
	// CPUWindow separa las dos muestras de CPU (default 250ms).
	CPUWindow time.Duration
}

const (
	diskBudget     = 1500 * time.Millisecond
	diskMaxEntries = 500_000
)

// Collect mide proceso, disco y tráfico. Nunca falla por la app estar
// detenida: devuelve el reporte con running=false.
func Collect(in Input) (Report, error) {
	win := in.Window
	if win <= 0 {
		win = 5 * time.Minute
	}
	rep := Report{
		App:      in.App.Name,
		Pid:      in.Status.Pid,
		Running:  in.Status.Running,
		HTTPOK:   in.Status.HTTPOK,
		Restarts: in.Status.Restarts,
		Traffic:  traffic(in, win),
	}

	if in.Status.Running && in.Status.Pid > 0 {
		first, err := procstat.Read(in.Status.Pid)
		if err != nil {
			return rep, fmt.Errorf("leer proceso %d: %w", in.Status.Pid, err)
		}
		pause := in.CPUWindow
		if pause <= 0 {
			pause = 250 * time.Millisecond
		}
		time.Sleep(pause)
		second, err := procstat.Read(in.Status.Pid)
		if err != nil {
			// el proceso murió entre muestras: reporte honesto, sin error
			rep.Running = false
			rep.Pid = 0
			rep.HTTPOK = false
		} else {
			rep.CPUPct = float64(second.CPUTime-first.CPUTime) / float64(pause) * 100
			rep.RSSMB = mb(second.RSSBytes)
			rep.PeakRSSMB = mb(second.PeakRSSBytes)
			rep.VirtualMB = mb(second.VirtualBytes)
			rep.IOReadMB = mb(second.ReadBytes)
			rep.IOWriteMB = mb(second.WriteBytes)
			rep.Handles = second.Handles
			rep.Threads = second.Threads
			if !second.StartTime.IsZero() {
				rep.UptimeS = int64(time.Since(second.StartTime).Seconds())
			}
		}
	}

	dir := in.App.Dir
	if dir == "" {
		dir = in.App.Static
	}
	rep.DiskBytes, rep.DiskSeen = dirSizeCached(dir)
	rep.LogBytes = fileSize(filepath.Join(in.LogsDir, "apps", in.App.Name+".log"))
	return rep, nil
}

// cache de tamaño de disco: caminar un árbol grande tarda; 30s basta.
var diskCache = struct {
	mu sync.Mutex
	m  map[string]diskEntry
}{m: map[string]diskEntry{}}

type diskEntry struct {
	at   time.Time
	size int64
	seen bool
}

func dirSizeCached(dir string) (int64, bool) {
	if dir == "" {
		return 0, true
	}
	diskCache.mu.Lock()
	e, ok := diskCache.m[dir]
	diskCache.mu.Unlock()
	if ok && time.Since(e.at) < 30*time.Second {
		return e.size, e.seen
	}
	size, seen := dirSize(dir)
	diskCache.mu.Lock()
	diskCache.m[dir] = diskEntry{at: time.Now(), size: size, seen: seen}
	diskCache.mu.Unlock()
	return size, seen
}

// traffic suma el access log de los hosts que sirven a esta app.
func traffic(in Input, win time.Duration) Traffic {
	t := Traffic{WindowS: int(win.Seconds())}
	hosts := hostsFor(in.Cfg, in.App)
	if len(hosts) == 0 {
		return t
	}
	cut := time.Now().Add(-win)
	for _, e := range weblog.ParseFile(filepath.Join(in.LogsDir, "access.log")) {
		if e.Time.IsZero() || e.Time.Before(cut) {
			continue
		}
		if !hosts[strings.ToLower(e.Host)] {
			continue
		}
		t.Requests++
		t.Bytes += e.Bytes
		if e.Status >= 500 {
			t.Errors++
		}
	}
	if t.Requests > 0 {
		t.RPS = float64(t.Requests) / win.Seconds()
		t.ErrPct = float64(t.Errors) / float64(t.Requests) * 100
	}
	return t
}

// hostsFor mapea rutas -> app por el puerto del target (PORT del env).
func hostsFor(cfg *config.Config, app config.App) map[string]bool {
	port := app.Env["PORT"]
	if port == "" || cfg == nil {
		return nil
	}
	out := map[string]bool{}
	for _, r := range cfg.Routes {
		u, err := url.Parse(r.Target)
		if err != nil || !strings.HasSuffix(u.Host, ":"+port) {
			continue
		}
		for _, h := range r.Hosts {
			out[strings.ToLower(h)] = true
		}
	}
	return out
}

// dirSize camina el dir con presupuesto de tiempo; seen=false si se cortó.
func dirSize(dir string) (int64, bool) {
	if dir == "" {
		return 0, true
	}
	deadline := time.Now().Add(diskBudget)
	var total int64
	entries := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entries++
		if entries > diskMaxEntries || time.Now().After(deadline) {
			return filepath.SkipAll
		}
		if !d.IsDir() {
			if info, ierr := d.Info(); ierr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, entries <= diskMaxEntries && time.Now().Before(deadline)
}

func fileSize(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.Size()
	}
	return 0
}

func mb(b uint64) float64 { return float64(b) / 1048576 }
