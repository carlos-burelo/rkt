package app

import (
	"path/filepath"
	"time"

	"rkt-proxy/internal/alerts"
	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/deploy"
	"rkt-proxy/internal/discover"
	"rkt-proxy/internal/github"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
	"rkt-proxy/internal/weblog"
)

// alertLoop evalúa reglas cada minuto: failed por app desde events,
// error_rate global desde access.log (entries no traen app).
// Disparo = evento kind=alert + cooldown de ventana.
func (p *Program) alertLoop(live *config.Live, db *store.DB) {
	_ = live
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		rules, err := db.ListRules()
		if err != nil || len(rules) == 0 {
			continue
		}
		now := time.Now().UTC()
		evs, _ := db.ListEvents(500, "", "")
		for _, r := range rules {
			since := now.Add(-time.Duration(r.WindowMin) * time.Minute)
			failed := 0
			for _, e := range evs {
				t, err := time.Parse(time.RFC3339, e.Ts)
				if err != nil || t.Before(since) {
					continue
				}
				if r.App != "*" && e.App != r.App {
					continue
				}
				if e.Kind == "died" || e.Kind == "failed" {
					failed++
				}
			}
			rate := 0.0
			if r.Kind == "error_rate" {
				total, errs := 0, 0
				for _, e := range weblog.ParseFile(filepath.Join(logx.DirFor(p.cfgPath), "access.log")) {
					if e.Time.Before(since) {
						continue
					}
					total++
					if e.Status >= 500 {
						errs++
					}
				}
				if total > 0 {
					rate = float64(errs) / float64(total)
				}
			}
			if alerts.Fired(r, failed, rate, now) {
				msg := alerts.Describe(r)
				if err := db.InsertEvent("alert", r.App, msg); err == nil {
					_ = db.MarkFired(r.ID, now)
					p.log.Warn("alerta disparada", "regla", r.ID, "msg", msg)
				}
			}
		}
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
