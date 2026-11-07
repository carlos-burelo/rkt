// Fleet stream: un solo SSE con la flota viva (status, deployments,
// events, metas) por tópico, cursor y replay. Reemplaza el polling 5s
// del dashboard: snapshot al conectar + publishes con seq. Misma auth
// por cookie que el resto de /api/v1 (EventSource la manda solo).
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"rkt-proxy/internal/appview"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/hub"
	"rkt-proxy/internal/netutil"
)

// Ventanas iguales que el polling al que reemplaza.
const (
	fleetDepsN   = 50
	fleetEventsN = 30
)

// snapshotFor arma el snapshot completo de un tópico (mismas formas que
// los GET sueltos, para que el cliente mezcle sin ramas).
func (d *Deps) snapshotFor(topic string) (any, bool) {
	switch topic {
	case hub.TopicStatus:
		apps := []health.AppStatus{}
		if d.Apps != nil {
			apps = d.Apps()
		}
		cfg := d.Live.Get()
		httpsPort := ""
		if cfg.ListenHTTPS != "" {
			httpsPort = netutil.Port(cfg.ListenHTTPS, "443")
		}
		return map[string]any{
			"version":    d.Version,
			"routes":     len(cfg.Routes),
			"apps":       apps,
			"http_port":  netutil.Port(cfg.ListenHTTP, "80"),
			"https_port": httpsPort,
		}, true
	case hub.TopicDeployments:
		deps, err := d.DB.ListDeployments("", fleetDepsN)
		if err != nil {
			return nil, false
		}
		return deps, true
	case hub.TopicEvents:
		evs, err := d.DB.ListEvents(fleetEventsN, "", "")
		if err != nil {
			return nil, false
		}
		return evs, true
	case hub.TopicMetas:
		live := d.Live.Get()
		out := []map[string]any{}
		for _, x := range live.Apps {
			out = append(out, appview.App(live, x))
		}
		return out, true
	}
	return nil, false
}

// parseTopics filtra tópicos pedidos (vacío = todos).
func parseTopics(q string) map[string]bool {
	if strings.TrimSpace(q) == "" {
		return nil
	}
	out := map[string]bool{}
	for _, t := range strings.Split(q, ",") {
		t = strings.TrimSpace(t)
		switch t {
		case hub.TopicStatus, hub.TopicDeployments, hub.TopicEvents, hub.TopicMetas:
			out[t] = true
		}
	}
	return out
}

// handleFleetStream sirve text/event-stream con snapshots o replay por
// cursor, hello final con seq, publishes en vivo y ping cada 15s
// (watchdog del cliente). Sin Hub (tests) responde 503.
func (d *Deps) handleFleetStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if d.Hub == nil {
		writeErr(w, http.StatusServiceUnavailable, "stream de flota deshabilitado")
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "stream no soportado")
		return
	}
	topics := parseTopics(r.URL.Query().Get("topics"))
	var cursor int64
	if s := strings.TrimSpace(r.URL.Query().Get("cursor")); s != "" {
		_, _ = fmt.Sscanf(s, "%d", &cursor)
		if cursor < 0 {
			cursor = 0
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, v any) {
		b, _ := json.Marshal(v)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		fl.Flush()
	}

	missed, seq, gap := d.Hub.Since(cursor, topics)
	resync := cursor == 0 || gap
	if resync {
		for _, t := range []string{hub.TopicStatus, hub.TopicDeployments, hub.TopicEvents, hub.TopicMetas} {
			if len(topics) > 0 && !topics[t] {
				continue
			}
			if snap, ok := d.snapshotFor(t); ok {
				send("snapshot", map[string]any{"seq": 0, "topic": t, "data": snap})
			}
		}
	} else {
		for _, e := range missed {
			send(e.Type, map[string]any{"seq": e.Seq, "topic": e.Topic, "data": e.Data})
		}
	}
	// Hello al final: marca "al día hasta seq". El cliente aplica
	// snapshots siempre (estado completo) y filtra live por seq.
	send("hello", map[string]any{"seq": seq, "resync": resync})

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastPing := time.Now()
	lastSeq := seq
	if cursor != 0 && !gap && len(missed) > 0 {
		lastSeq = missed[len(missed)-1].Seq
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			evs, cur, _ := d.Hub.Since(lastSeq, topics)
			for _, e := range evs {
				send(e.Type, map[string]any{"seq": e.Seq, "topic": e.Topic, "data": e.Data})
				lastSeq = e.Seq
			}
			_ = cur
			if time.Since(lastPing) > 15*time.Second {
				send("ping", map[string]any{"t": time.Now().UTC().Format(time.RFC3339)})
				lastPing = time.Now()
			}
		}
	}
}
