package api

import (
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/weblog"
)

// entries lee access.log filtrado por ventana (cero = todo).
func entries(logsDir string, since time.Time, host string) []weblog.Entry {
	all := weblog.ParseFile(filepath.Join(logsDir, "access.log"))
	if since.IsZero() && host == "" {
		return all
	}
	out := all[:0]
	for _, e := range all {
		if !since.IsZero() && e.Time.Before(since) {
			continue
		}
		if host != "" && e.Host != host {
			continue
		}
		out = append(out, e)
	}
	return out
}

// handleUsage rollup de 30d (caché 5min). Sin cobro: solo uso medido.
func (d *Deps) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	days := queryInt(r, "days", 30)
	b, err := d.cache.get("usage:"+strconv.Itoa(days), 5*time.Minute, func() (any, error) {
		es := entries(d.LogsDir, time.Now().AddDate(0, 0, -days), "")
		s := weblog.Summarize(es)
		errs := s.ByClass["5xx"]
		rate := 0.0
		if s.Total > 0 {
			rate = float64(errs) / float64(s.Total)
		}
		return map[string]any{
			"days": days, "total": s.Total, "bytes": s.Bytes,
			"err_rate": rate, "p95": s.P95, "by_day": s.ByDay,
		}, nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// handleAnalytics por host (o global): requests, bytes, errores, p50/p95,
// top rutas/referers, serie diaria. Sin visitors/bounce inventados.
func (d *Deps) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	host := r.URL.Query().Get("host")
	days := queryInt(r, "days", 7)
	key := "analytics:" + host + ":" + strconv.Itoa(days)
	b, err := d.cache.get(key, 5*time.Minute, func() (any, error) {
		es := entries(d.LogsDir, time.Now().AddDate(0, 0, -days), host)
		return weblog.Summarize(es), nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// handleObservability serie req/errores por hora + latencia, clases,
// tráfico por app, restarts y caídas. Todo derivado de access.log/events.
func (d *Deps) handleObservability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	hours := queryInt(r, "hours", 6)
	if hours > 168 {
		hours = 168
	}
	b, err := d.cache.get("obs:"+strconv.Itoa(hours), time.Minute, func() (any, error) {
		since := time.Now().Add(-time.Duration(hours) * time.Hour)
		es := entries(d.LogsDir, since, "")
		type bucket struct {
			Ts  string `json:"ts"`
			Req int    `json:"req"`
			Err int    `json:"err"`
			P50 string `json:"p50,omitempty"`
			P95 string `json:"p95,omitempty"`
			P99 string `json:"p99,omitempty"`
		}
		type appObs struct {
			App   string `json:"app"`
			Req   int    `json:"req"`
			Err   int    `json:"err"`
			Bytes int64  `json:"bytes"`
			P95   string `json:"p95,omitempty"`
		}
		buckets := make([]bucket, hours)
		durs := make([][]time.Duration, hours)
		for i := range buckets {
			buckets[i].Ts = since.Add(time.Duration(i+1) * time.Hour).Format("15:04")
		}
		classes := map[string]int{}
		var allDurs []time.Duration
		hostApp := map[string]string{}
		if d.Live != nil {
			for _, rt := range d.Live.Get().Routes {
				owner := appview.Owner(d.Live.Get(), rt.Target)
				for _, h := range rt.Hosts {
					hostApp[strings.ToLower(h)] = owner
				}
			}
		}
		appMap := map[string]*appObs{}
		appDurs := map[string][]time.Duration{}
		for _, e := range es {
			i := int(e.Time.Sub(since).Hours())
			if i >= 0 && i < hours {
				buckets[i].Req++
				if e.Status >= 500 {
					buckets[i].Err++
				}
				if e.Dur > 0 {
					durs[i] = append(durs[i], e.Dur)
					allDurs = append(allDurs, e.Dur)
				}
			}
			classes[statusClass(e.Status)]++
			app := hostApp[strings.ToLower(e.Host)]
			if app == "" {
				continue
			}
			ao := appMap[app]
			if ao == nil {
				ao = &appObs{App: app}
				appMap[app] = ao
			}
			ao.Req++
			ao.Bytes += e.Bytes
			if e.Status >= 500 {
				ao.Err++
			}
			if e.Dur > 0 {
				appDurs[app] = append(appDurs[app], e.Dur)
			}
		}
		for i := range buckets {
			buckets[i].P50, buckets[i].P95, buckets[i].P99 = percentiles(durs[i])
		}
		p50, p95, p99 := percentiles(allDurs)
		byApp := make([]appObs, 0, len(appMap))
		for name, ao := range appMap {
			_, ao.P95, _ = percentiles(appDurs[name])
			byApp = append(byApp, *ao)
		}
		sort.Slice(byApp, func(i, j int) bool { return byApp[i].Req > byApp[j].Req })
		restarts := 0
		if evs, err := d.DB.ListEvents(500, "", ""); err == nil {
			for _, e := range evs {
				t, err := time.Parse(time.RFC3339, e.Ts)
				if err != nil || t.Before(since) {
					continue
				}
				if e.Kind == "died" || e.Kind == "restarted" || e.Kind == "failed" {
					restarts++
				}
			}
		}
		down := []string{}
		if d.Apps != nil {
			for _, a := range d.Apps() {
				if !a.Running || a.Failed {
					down = append(down, a.Name)
				}
			}
		}
		return map[string]any{
			"hours": hours, "buckets": buckets, "classes": classes,
			"by_app": byApp, "restarts": restarts, "down": down,
			"p50": p50, "p95": p95, "p99": p99,
		}, nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500:
		return "5xx"
	default:
		return "other"
	}
}

func percentiles(ds []time.Duration) (string, string, string) {
	if len(ds) == 0 {
		return "", "", ""
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	at := func(p float64) string {
		i := int(float64(len(ds)) * p)
		if i >= len(ds) {
			i = len(ds) - 1
		}
		return ds[i].String()
	}
	return at(0.50), at(0.95), at(0.99)
}

// handleProxy muestra el alcance real de cada ruta: backend, TLS y DNS.
func (d *Deps) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	type routeStatus struct {
		Hosts  []string `json:"hosts"`
		Target string   `json:"target"`
		OK     bool     `json:"ok"`
		TLS    []string `json:"tls"`
		DNSOK  []bool   `json:"dns_ok"`
	}
	out := []routeStatus{}
	for _, x := range d.Live.Get().Routes {
		rs := routeStatus{Hosts: x.Hosts, Target: x.Target, TLS: []string{}, DNSOK: []bool{}}
		if u, err := url.Parse(x.Target); err == nil && u.Host != "" {
			rs.OK = health.BackendOK(u.Host)
		}
		for _, h := range x.Hosts {
			state := "off"
			if d.TLS != nil {
				if ok, exp := d.TLS(h); ok {
					state = exp
					if state == "" {
						state = "auto"
					}
				}
			}
			rs.TLS = append(rs.TLS, state)
			rs.DNSOK = append(rs.DNSOK, appview.ResolvesLocal(h))
		}
		out = append(out, rs)
	}
	writeJSON(w, out)
}
