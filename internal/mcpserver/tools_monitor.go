// Tools de monitoreo en tiempo real: snapshots con métricas, long-poll de
// estado/eventos/deployments y seguimiento de logs.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt-proxy/internal/appusage"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/procstat"
	"rkt-proxy/internal/weblog"
)

type watchStatusArg struct {
	App      string `json:"app,omitempty" jsonschema:"app a vigilar (vacío = cualquier cambio)"`
	Want     string `json:"want,omitempty" jsonschema:"healthy|unhealthy|running|stopped|failed (default healthy)"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 30, máx 300)"`
}
type watchEventsArg struct {
	App      string `json:"app,omitempty" jsonschema:"filtrar por app"`
	Kind     string `json:"kind,omitempty" jsonschema:"died|failed|alert|deploy…"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 30, máx 300)"`
}
type watchDeployArg struct {
	App      string `json:"app,omitempty" jsonschema:"app a vigilar"`
	AfterID  int64  `json:"after_id,omitempty" jsonschema:"id base; espera deployments mayores"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 120, máx 600)"`
}
type tailFollowArg struct {
	App      string `json:"app,omitempty" jsonschema:"app (vacío = operativo)"`
	Pattern  string `json:"pattern,omitempty" jsonschema:"regex a esperar (vacío = devuelve lo nuevo)"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 15, máx 120)"`
	MaxLines int    `json:"max_lines,omitempty" jsonschema:"tope de líneas (default 200)"`
}
type trafficArg struct {
	Seconds int `json:"seconds,omitempty" jsonschema:"ventana en segundos (default 60, máx 86400)"`
}
type appUsageArg struct {
	App     string `json:"app" jsonschema:"app a medir"`
	WindowS int    `json:"window_s,omitempty" jsonschema:"ventana de tráfico (default 300, máx 86400)"`
}

type healthApp struct {
	Name     string `json:"name"`
	Running  bool   `json:"running"`
	Pid      int    `json:"pid"`
	Restarts int    `json:"restarts"`
	Failed   bool   `json:"failed"`
	HTTPOK   bool   `json:"http_ok"`
}

type healthSnap struct {
	Status   string      `json:"status"`
	Version  string      `json:"version"`
	Routes   int         `json:"routes"`
	Apps     []healthApp `json:"apps"`
	Backends []struct {
		Target string `json:"target"`
		OK     bool   `json:"ok"`
	} `json:"backends"`
}

func (s *Server) healthSnap() (*healthSnap, error) {
	cfg, err := cfgfile.Load(s.cfgPath)
	if err != nil {
		return nil, err
	}
	if !cfg.Healthz.Enabled {
		return nil, fmt.Errorf("healthz desactivado")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + cfg.Healthz.Listen + "/healthz")
	if err != nil {
		return nil, fmt.Errorf("sin servicio: %w", err)
	}
	defer resp.Body.Close()
	var snap healthSnap
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func stateOfApp(a healthApp) string {
	switch {
	case a.Failed:
		return "failed"
	case a.Running && !a.HTTPOK:
		return "degrado"
	case a.Running:
		return "healthy"
	default:
		return "stopped"
	}
}

func wantMatches(a healthApp, want string) bool {
	switch want {
	case "", "healthy":
		return a.Running && a.HTTPOK && !a.Failed
	case "unhealthy":
		return !a.Running || !a.HTTPOK || a.Failed
	case "running":
		return a.Running
	case "stopped":
		return !a.Running
	case "failed":
		return a.Failed
	default:
		return a.Running
	}
}

func (s *Server) doWatchStatus(a watchStatusArg) (string, error) {
	timeout := a.TimeoutS
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	baseline := ""
	for {
		snap, err := s.healthSnap()
		if err == nil {
			if a.App == "" {
				sig := fmt.Sprint(snap.Apps)
				if baseline == "" {
					baseline = sig
				} else if sig != baseline && a.Want == "" {
					return jsonText(map[string]any{"changed": true, "apps": snap.Apps, "status": snap.Status})
				}
			}
			for _, ap := range snap.Apps {
				if a.App != "" && ap.Name != a.App {
					continue
				}
				if wantMatches(ap, a.Want) {
					return jsonText(map[string]any{
						"matched": true, "app": ap.Name, "state": stateOfApp(ap),
						"pid": ap.Pid, "restarts": ap.Restarts, "http_ok": ap.HTTPOK,
					})
				}
			}
		}
		if time.Now().After(deadline) {
			return jsonText(map[string]any{"matched": false, "timeout": true, "want": a.Want})
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Server) doWatchEvents(a watchEventsArg) (string, error) {
	timeout := a.TimeoutS
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	base, err := db.ListEvents(1, "", "")
	if err != nil {
		return "", err
	}
	var baseID int64
	if len(base) > 0 {
		baseID = base[0].ID
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for {
		evs, err := db.ListEvents(200, a.Kind, a.App)
		if err != nil {
			return "", err
		}
		fresh := []any{}
		for _, e := range evs {
			if e.ID > baseID {
				fresh = append(fresh, e)
			}
		}
		if len(fresh) > 0 {
			return jsonText(map[string]any{"events": fresh, "count": len(fresh)})
		}
		if time.Now().After(deadline) {
			return jsonText(map[string]any{"events": []any{}, "timeout": true})
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Server) doWatchDeployment(a watchDeployArg) (string, error) {
	timeout := a.TimeoutS
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	after := a.AfterID
	if after == 0 {
		deps, err := db.ListDeployments("", 1)
		if err != nil {
			return "", err
		}
		if len(deps) > 0 {
			after = deps[0].ID
		}
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for {
		deps, err := db.ListDeployments(a.App, 50)
		if err != nil {
			return "", err
		}
		for _, d := range deps {
			if d.ID <= after {
				continue
			}
			if d.Status != "running" {
				return jsonText(map[string]any{"deployment": d, "finished": true})
			}
		}
		if time.Now().After(deadline) {
			return jsonText(map[string]any{"finished": false, "timeout": true})
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Server) doTailFollow(a tailFollowArg) (string, error) {
	timeout := a.TimeoutS
	if timeout <= 0 {
		timeout = 15
	}
	if timeout > 120 {
		timeout = 120
	}
	maxLines := a.MaxLines
	if maxLines <= 0 {
		maxLines = 200
	}
	path := logx.Path(s.cfgPath)
	if a.App != "" {
		path = fmt.Sprintf("%s/apps/%s.log", logx.DirFor(s.cfgPath), a.App)
	}
	var rx *regexp.Regexp
	if a.Pattern != "" {
		var err error
		rx, err = regexp.Compile(a.Pattern)
		if err != nil {
			return "", fmt.Errorf("regex inválida: %w", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil {
		_, _ = f.Seek(st.Size(), io.SeekStart)
	}
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	lines := []string{}
	buf := make([]byte, 4096)
	partial := ""
	for {
		n, _ := f.Read(buf)
		if n > 0 {
			partial += string(buf[:n])
			parts := strings.Split(partial, "\n")
			partial = parts[len(parts)-1]
			for _, line := range parts[:len(parts)-1] {
				if rx == nil || rx.MatchString(line) {
					lines = append(lines, line)
				}
			}
			if len(lines) > maxLines {
				lines = lines[len(lines)-maxLines:]
			}
			if rx != nil && len(lines) > 0 {
				return jsonText(map[string]any{"lines": lines, "matched": true})
			}
		}
		if time.Now().After(deadline) {
			return jsonText(map[string]any{"lines": lines, "matched": len(lines) > 0, "timeout": true})
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (s *Server) doProcessStats(app string) (string, error) {
	snap, err := s.healthSnap()
	if err != nil {
		return "", err
	}
	for _, a := range snap.Apps {
		if a.Name != app {
			continue
		}
		if a.Pid == 0 {
			return "", fmt.Errorf("app %s sin pid (detenida)", app)
		}
		first, err := procstat.Read(a.Pid)
		if err != nil {
			return "", err
		}
		time.Sleep(500 * time.Millisecond)
		second, err := procstat.Read(a.Pid)
		if err != nil {
			return "", err
		}
		cpuPct := float64(second.CPUTime-first.CPUTime) / float64(500*time.Millisecond) * 100
		return jsonText(map[string]any{
			"app": app, "pid": a.Pid, "state": stateOfApp(a),
			"rss_mb":   float64(second.RSSBytes) / 1048576,
			"cpu_pct":  cpuPct,
			"restarts": a.Restarts, "http_ok": a.HTTPOK,
		})
	}
	return "", fmt.Errorf("app %q no está en la flota", app)
}

func (s *Server) doTrafficRate(a trafficArg) (string, error) {
	sec := a.Seconds
	if sec <= 0 {
		sec = 60
	}
	if sec > 86400 {
		sec = 86400
	}
	cut := time.Now().Add(-time.Duration(sec) * time.Second)
	all := weblog.ParseFile(logx.AccessPath(s.cfgPath))
	total, errs, bytes := 0, 0, int64(0)
	durs := []int64{}
	for _, e := range all {
		if e.Time.IsZero() || e.Time.Before(cut) {
			continue
		}
		total++
		bytes += e.Bytes
		if e.Status >= 500 {
			errs++
		}
		if e.Dur > 0 {
			durs = append(durs, int64(e.Dur))
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p := func(q float64) string {
		if len(durs) == 0 {
			return "—"
		}
		return time.Duration(durs[int(float64(len(durs)-1)*q)]).String()
	}
	errPct := 0.0
	if total > 0 {
		errPct = float64(errs) / float64(total) * 100
	}
	return jsonText(map[string]any{
		"window_s": sec, "requests": total, "rps": float64(total) / float64(sec),
		"errors": errs, "err_pct": errPct,
		"bytes": bytes, "bytes_s": float64(bytes) / float64(sec),
		"p50": p(0.50), "p95": p(0.95), "p99": p(0.99),
	})
}

func (s *Server) doMetricsNow() (string, error) {
	snap, err := s.healthSnap()
	if err != nil {
		return "", err
	}
	type row struct {
		Name     string  `json:"name"`
		State    string  `json:"state"`
		Pid      int     `json:"pid"`
		Restarts int     `json:"restarts"`
		HTTPOK   bool    `json:"http_ok"`
		RSSMB    float64 `json:"rss_mb,omitempty"`
	}
	rows := []row{}
	for _, a := range snap.Apps {
		r := row{Name: a.Name, State: stateOfApp(a), Pid: a.Pid, Restarts: a.Restarts, HTTPOK: a.HTTPOK}
		if a.Pid > 0 {
			if smp, err := procstat.Read(a.Pid); err == nil {
				r.RSSMB = float64(smp.RSSBytes) / 1048576
			}
		}
		rows = append(rows, r)
	}
	traffic, _ := s.doTrafficRate(trafficArg{Seconds: 300})
	return jsonText(map[string]any{
		"status": snap.Status, "version": snap.Version, "routes": snap.Routes,
		"apps": rows, "traffic_5m": json.RawMessage(traffic),
	})
}

// doAppUsage mide proceso + disco + tráfico real de una app.
func (s *Server) doAppUsage(a appUsageArg) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	app := cfgfile.FindApp(cfg, a.App)
	if app == nil {
		return "", fmt.Errorf("app %q inexistente", a.App)
	}
	snap, err := s.healthSnap()
	if err != nil {
		return "", err
	}
	st := health.AppStatus{Name: app.Name}
	for _, x := range snap.Apps {
		if x.Name == app.Name {
			st = health.AppStatus{Name: x.Name, Running: x.Running, Pid: x.Pid, Restarts: x.Restarts, Failed: x.Failed, HTTPOK: x.HTTPOK}
		}
	}
	win := a.WindowS
	if win <= 0 {
		win = 300
	}
	if win > 86400 {
		win = 86400
	}
	rep, err := appusage.Collect(appusage.Input{
		App: *app, Cfg: cfg, LogsDir: logx.DirFor(s.cfgPath),
		Status: st, Window: time.Duration(win) * time.Second,
	})
	if err != nil {
		return "", err
	}
	return jsonText(rep)
}

// registerMonitor añade tools de monitoreo en vivo.
func (s *Server) registerMonitor(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "metrics_now", Description: "Foto actual: estado/pid/RSS por app + tráfico de 5 min (req, errores, p50/p95/p99)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doMetricsNow())
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "process_stats", Description: "RSS y %CPU (muestra 500ms) de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doProcessStats(a.App))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "traffic_rate", Description: "Requests/s, errores, bytes y percentiles en una ventana de segundos."},
		func(ctx context.Context, req *mcp.CallToolRequest, a trafficArg) (*mcp.CallToolResult, any, error) {
			return text(s.doTrafficRate(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "watch_status", Description: "Long-poll: bloquea hasta que la app (o cualquier cambio) cumpla healthy/unhealthy/running/stopped/failed."},
		func(ctx context.Context, req *mcp.CallToolRequest, a watchStatusArg) (*mcp.CallToolResult, any, error) {
			return text(s.doWatchStatus(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "watch_events", Description: "Long-poll: devuelve eventos nuevos (died/failed/alert/deploy) en cuanto aparecen."},
		func(ctx context.Context, req *mcp.CallToolRequest, a watchEventsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doWatchEvents(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "watch_deployment", Description: "Long-poll: espera a que un deployment nuevo (o after_id) termine ok/failed."},
		func(ctx context.Context, req *mcp.CallToolRequest, a watchDeployArg) (*mcp.CallToolResult, any, error) {
			return text(s.doWatchDeployment(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "tail_follow", Description: "Sigue un log desde el final: devuelve líneas nuevas, o espera hasta que una regex aparezca."},
		func(ctx context.Context, req *mcp.CallToolRequest, a tailFollowArg) (*mcp.CallToolResult, any, error) {
			return text(s.doTailFollow(a))
		})
	mcp.AddTool(srv, &mcp.Tool{Name: "app_usage", Description: "Consumo real de una app: CPU%, RSS/pico/virtual, I/O, handles/hilos, disco del proyecto, log y tráfico de la ventana."},
		func(ctx context.Context, req *mcp.CallToolRequest, a appUsageArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppUsage(a))
		})
}
