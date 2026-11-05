package api

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
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

// handleObservability serie req/errores por hora + restarts + caídas.
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
		}
		buckets := make([]bucket, hours)
		for i := range buckets {
			buckets[i].Ts = since.Add(time.Duration(i+1) * time.Hour).Format("15:04")
		}
		for _, e := range es {
			i := int(e.Time.Sub(since).Hours())
			if i < 0 || i >= hours {
				continue
			}
			buckets[i].Req++
			if e.Status >= 500 {
				buckets[i].Err++
			}
		}
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
			"hours": hours, "buckets": buckets,
			"restarts": restarts, "down": down,
		}, nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
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
