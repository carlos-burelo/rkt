package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"rkt-proxy/internal/alerts"
	"rkt-proxy/internal/appusage"
	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/procstat"
	"rkt-proxy/internal/servermetrics"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/weblog"
)

// alertLoop evalúa reglas cada minuto con datos reales (events,
// access.log, supervisor) y entrega webhooks con reintentos.
func (p *Program) alertLoop(live *config.Live, db *store.DB) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		rules, err := db.ListRules()
		if err != nil || len(rules) == 0 {
			continue
		}
		now := time.Now().UTC()
		evs, _ := db.ListEvents(500, "", "")
		entries := weblog.ParseFile(filepath.Join(logx.DirFor(p.cfgPath), "access.log"))
		cfg := live.Get()
		statuses := map[string]int{}
		for _, s := range p.sup.Snapshot() {
			statuses[s.Name] = s.Pid
		}
		srv := servermetrics.Snapshot{}
		if p.srvMetrics != nil {
			srv, _ = p.srvMetrics.Snapshot()
		}
		for _, r := range rules {
			if !r.Enabled {
				continue
			}
			m := p.metricsFor(r, cfg, entries, evs, statuses, now, srv)
			if alerts.Fired(r, m, now) {
				msg := alerts.Describe(r, m)
				fireID, err := db.AddFire(r.ID, m.Value(r.Kind), r.Threshold, msg)
				if err == nil {
					_ = db.InsertEvent("alert", r.App, msg)
					_ = db.MarkFired(r.ID, now)
					if r.ChannelID > 0 {
						_, _ = db.EnqueueDelivery(fireID, r.ChannelID)
					}
					p.log.Warn("alerta disparada", "regla", r.ID, "msg", msg)
				}
			}
		}
		p.deliverAlerts(db, now)
	}
}

func (p *Program) metricsFor(r store.AlertRule, cfg *config.Config, entries []weblog.Entry, evs []store.Event, statuses map[string]int, now time.Time, srv servermetrics.Snapshot) alerts.Metrics {
	since := now.Add(-time.Duration(r.WindowMin) * time.Minute)
	var m alerts.Metrics
	m.CPUPct = srv.CPU.Percent
	m.MemPct = srv.Memory.Percent
	for _, t := range srv.Temps {
		if t.Celsius > m.TempC {
			m.TempC = t.Celsius
		}
	}
	for _, d := range srv.Disks {
		if d.Percent > m.DiskPct {
			m.DiskPct = d.Percent
		}
	}
	hosts := map[string]bool{}
	if r.App != "*" {
		if a := cfgfile.FindApp(cfg, r.App); a != nil {
			for _, h := range appview.Domains(cfg, *a) {
				hosts[h] = true
			}
		}
	}
	var durs []time.Duration
	for _, e := range entries {
		if e.Time.Before(since) {
			continue
		}
		if r.App != "*" && !hosts[e.Host] {
			continue
		}
		if e.Status >= 500 {
			m.Failed++
		}
		if e.Dur > 0 {
			durs = append(durs, e.Dur)
		}
	}
	if m.Failed > 0 {
		// error_rate usa el total de la misma ventana, no solo 5xx.
		total := 0
		for _, e := range entries {
			if e.Time.After(since) && (r.App == "*" || hosts[e.Host]) {
				total++
			}
		}
		if total > 0 {
			m.ErrRate = float64(m.Failed) / float64(total)
		}
	}
	for _, e := range evs {
		t, err := time.Parse(time.RFC3339, e.Ts)
		if err != nil || t.Before(since) || (r.App != "*" && e.App != r.App) {
			continue
		}
		if e.Kind == "died" || e.Kind == "failed" {
			m.Failed++
		}
	}
	if len(durs) > 0 {
		sortDurations(durs)
		i := int(float64(len(durs)) * 0.95)
		if i >= len(durs) {
			i = len(durs) - 1
		}
		m.P95ms = float64(durs[i].Milliseconds())
	}
	if r.App != "*" {
		if pid := statuses[r.App]; pid > 0 {
			if s, err := procstat.Read(pid); err == nil {
				m.MemoryMB = float64(s.RSSBytes) / 1048576
			}
		} else {
			m.DownMin = r.WindowMin
		}
		if quota, _ := strconv.ParseInt(p.db.GetSetting(store.KeyQuotaBytes), 10, 64); quota > 0 {
			if a := cfgfile.FindApp(cfg, r.App); a != nil {
				if rep, err := appusage.Collect(appusage.Input{App: *a, Cfg: cfg, LogsDir: logx.DirFor(p.cfgPath)}); err == nil {
					m.DiskPct = float64(rep.DiskBytes) / float64(quota) * 100
				}
			}
		}
	}
	return m
}

func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

func (p *Program) deliverAlerts(db *store.DB, now time.Time) {
	pending, err := db.PendingDeliveries(20)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, d := range pending {
		ts, _ := time.Parse(time.RFC3339, d.NextAt)
		if d.NextAt != "" && now.Before(ts) {
			continue
		}
		body, _ := json.Marshal(map[string]any{
			"source": "rkt", "fire_id": d.FireID, "channel_id": d.ChannelID, "ts": now.Format(time.RFC3339),
		})
		req, err := http.NewRequest(http.MethodPost, d.URL, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		attempts := d.Attempts + 1
		if err != nil || resp.StatusCode >= 300 {
			msg := "sin respuesta"
			if err != nil {
				msg = err.Error()
			} else {
				resp.Body.Close()
				msg = "http " + strconv.Itoa(resp.StatusCode)
			}
			status := "pending"
			next := now.Add(time.Duration(attempts*30) * time.Second).Format(time.RFC3339)
			if attempts >= 5 {
				status, next = "failed", ""
			}
			_ = db.MarkDelivery(d.ID, status, attempts, next, msg)
			continue
		}
		resp.Body.Close()
		_ = db.MarkDelivery(d.ID, "sent", attempts, "", "")
	}
}

// runDiscovery registra repos nuevos de todas las instalaciones.
func (p *Program) runDiscovery() {
	if p.db == nil || p.gh() == nil {
		return
	}
	if _, err := discover.Run(discover.Deps{
		GH: p.gh(), DB: p.db, Log: p.log,
		DeployFn: func(name, actor string) (string, error) { return p.deployApp(name, actor) },
	}); err != nil {
		p.log.Warn("discovery fallida", "err", err)
	}
}

// autoDeploy sondea GitHub por cambios y despliega (push-to-deploy con
// delay de poll). El git local es el estado: si HEAD != remoto, deploy.
// El intervalo se recalcula: tras crear la App la config llega por hot-reload.
func (p *Program) autoDeploy(live *config.Live) {
	next := 2 * time.Second
	var lastPoll, lastDiscovery time.Time
	forceDiscovery := false
	for {
		wait := next
		if gh := live.Get().Github; gh != nil && gh.Poll() > 0 && !lastPoll.IsZero() {
			wait = gh.Poll()
		}
		select {
		case <-time.After(wait):
		case <-p.discoverKick:
			forceDiscovery = true
		}
		cfg := live.Get()
		if cfg.Github == nil || p.gh() == nil {
			next = 2 * time.Second
			continue
		}
		// 1) push-to-deploy primero: 1 request por app, latencia = poll.
		for i := range cfg.Apps {
			a := &cfg.Apps[i]
			if a.Deploy == nil || !a.Deploy.Auto {
				continue
			}
			owner, repo, ok := github.ParseRepo(a.Deploy.Repo)
			if !ok {
				p.log.Warn("auto-deploy: repo no parseable", "app", a.Name)
				continue
			}
			tok, err := p.appToken(a)
			if err != nil {
				p.log.Warn("auto-deploy: sin token", "app", a.Name, "err", err)
				continue
			}
			remote, err := p.gh().CommitSHA(owner, repo, a.Deploy.Ref, tok)
			if err != nil {
				p.log.Warn("auto-deploy: sin remoto", "app", a.Name, "err", err)
				continue
			}
			if local := deploy.HeadRev(a.Dir); local == "" || local == remote {
				continue
			}
			p.log.Info("auto-deploy: cambio detectado", "app", a.Name, "rev", shortSHA(remote))
			if rev, err := p.deployApp(a.Name, "auto"); err != nil {
				p.log.Error("auto-deploy fallido", "app", a.Name, "err", err)
			} else {
				p.log.Info("auto-deploy ok", "app", a.Name, "rev", rev)
			}
			cfg = live.Get()
		}
		// 2) auto-registro de repos nuevos: cada 5 min o kick (setup/discover).
		if forceDiscovery || time.Since(lastDiscovery) >= 5*time.Minute {
			p.runDiscovery()
			lastDiscovery = time.Now()
			forceDiscovery = false
		}
		lastPoll = time.Now()
		if cfg.Github.Poll() > 0 {
			next = cfg.Github.Poll()
		} else {
			next = 10 * time.Second
		}
	}
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// watchMerged recarga la flota cuando cambia la DB (única fuente).
// Fuente inválida = se conserva la anterior.
func (p *Program) watchMerged(live *config.Live, db *store.DB) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	lastVer := db.ConfigVersion()
	for range t.C {
		if v := db.ConfigVersion(); v == lastVer {
			continue
		} else {
			lastVer = v
		}
		base, err := cfgfile.LoadBase(db, p.cfgPath)
		if err != nil {
			p.log.Warn("infra inválida en DB, conservo anterior", "err", err)
			continue
		}
		next, err := cfgfile.Snapshot(db, base)
		if err != nil {
			p.log.Warn("snapshot inválido, conservo anterior", "err", err)
			continue
		}
		if err := cfgfile.OverlayGithub(next, db); err != nil {
			p.log.Warn("github en DB inválido, conservo anterior", "err", err)
			continue
		}
		old := live.Get()
		live.Swap(next)
		if p.sup != nil {
			// manifiesto en caliente: nuevas/actualizadas arrancan solas
			p.sup.Apply(next.Apps, next.GlobalEnv)
		}
		if next.Github != nil && p.gh() == nil {
			if gh, err := github.New(next.Github.AppID, next.Github.KeyFile, ""); err == nil {
				p.ghm.Store(gh)
			}
		}
		if next.Github != nil && p.autoStarted.CompareAndSwap(false, true) {
			go p.autoDeploy(live)
		}
		p.log.Info("config recargada",
			"routes", len(next.Routes),
			"apps", len(next.Apps),
			"redirect", next.RedirectHTTPToHTTPS,
		)
		if need := config.NeedRestart(old, next); len(need) > 0 {
			p.log.Warn("cambios requieren restart", "campos", need)
		}
	}
}
