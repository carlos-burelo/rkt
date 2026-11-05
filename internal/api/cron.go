package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
)

type cronView struct {
	App      string         `json:"app"`
	Job      string         `json:"job"`
	Schedule string         `json:"schedule"`
	Command  string         `json:"command"`
	Timeout  string         `json:"timeout,omitempty"`
	Disabled bool           `json:"disabled"`
	Running  bool           `json:"running"`
	Next     string         `json:"next,omitempty"`
	Last     *storeCronLast `json:"last,omitempty"`
}

type storeCronLast struct {
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	ExitCode   int    `json:"exit_code"`
	Message    string `json:"message"`
}

// handleCron GET /cron lista todas las tareas.
func (d *Deps) handleCron(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	cfg := d.Live.Get()
	out := []cronView{}
	for _, app := range cfg.Apps {
		for _, job := range app.Cron {
			out = append(out, d.cronView(app, job))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].App == out[j].App {
			return out[i].Job < out[j].Job
		}
		return out[i].App < out[j].App
	})
	writeJSON(w, out)
}

func (d *Deps) cronView(app config.App, job config.CronJob) cronView {
	v := cronView{
		App: app.Name, Job: job.Name, Schedule: job.Schedule,
		Command: job.Command, Timeout: job.Timeout, Disabled: job.Disabled,
	}
	if d.Cron != nil {
		v.Running = d.Cron.Running(app.Name, job.Name)
	}
	if sched, err := config.ParseCron(job.Schedule); err == nil {
		v.Next = sched.Next(time.Now()).UTC().Format(time.RFC3339)
	}
	if run, ok, err := d.DB.LastCronRun(app.Name, job.Name); err == nil && ok {
		v.Last = &storeCronLast{
			Status: run.Status, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
			ExitCode: run.ExitCode, Message: run.Message,
		}
	}
	return v
}

// handleCronDetail enruta /cron/runs, /cron/runs/{id}/log y
// /cron/{app}/{job}[/run].
func (d *Deps) handleCronDetail(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/cron/"), "/")
	if rest == "runs" {
		d.handleCronRuns(w, r)
		return
	}
	if strings.HasPrefix(rest, "runs/") {
		d.handleCronRunLog(w, r, strings.TrimPrefix(rest, "runs/"))
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || len(parts) > 3 {
		writeErr(w, http.StatusNotFound, "ruta de cron inválida")
		return
	}
	app, job, action := parts[0], parts[1], ""
	if len(parts) == 3 {
		action = parts[2]
	}
	switch {
	case action == "" && r.Method == http.MethodPut:
		d.putCron(w, r, app, job)
	case action == "" && r.Method == http.MethodDelete:
		if !requireOperator(w, r) {
			return
		}
		if err := d.mutateFleet(true, func(cp *config.Config) error {
			return cfgfile.DeleteCronJob(cp, app, job)
		}); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"ok": "true"})
	case action == "run" && r.Method == http.MethodPost:
		if !requireOperator(w, r) {
			return
		}
		if d.Cron == nil {
			writeErr(w, http.StatusServiceUnavailable, "cron no disponible")
			return
		}
		id, err := d.Cron.RunNow(app, job)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]int64{"run": id})
	case action == "runs" && r.Method == http.MethodGet:
		d.handleRunsFor(w, r, app, job)
	default:
		writeErr(w, http.StatusNotFound, "ruta de cron inválida")
	}
}

func (d *Deps) putCron(w http.ResponseWriter, r *http.Request, app, job string) {
	if !requireOperator(w, r) {
		return
	}
	var in struct {
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
		Timeout  string `json:"timeout"`
		Disabled bool   `json:"disabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	cj := config.CronJob{
		Name: job, Schedule: in.Schedule, Command: in.Command,
		Timeout: in.Timeout, Disabled: in.Disabled,
	}
	if err := d.mutateFleet(true, func(cp *config.Config) error {
		return cfgfile.SetCronJob(cp, app, cj)
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}

func (d *Deps) handleCronRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	d.handleRunsFor(w, r, r.URL.Query().Get("app"), r.URL.Query().Get("job"))
}

func (d *Deps) handleRunsFor(w http.ResponseWriter, r *http.Request, app, job string) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	runs, err := d.DB.ListCronRuns(app, job, queryInt(r, "n", 50))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, runs)
}

func (d *Deps) handleCronRunLog(w http.ResponseWriter, r *http.Request, raw string) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSuffix(raw, "/log"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	run, ok, err := d.DB.CronRunByID(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok || run.LogPath == "" {
		writeErr(w, http.StatusNotFound, "sin log para esa ejecución")
		return
	}
	out, err := logx.Tail(run.LogPath, queryInt(r, "n", 300))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]any{"log": out})
}
