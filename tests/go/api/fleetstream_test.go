package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"rkt-proxy/internal/hub"
)

func fleetServer(t *testing.T, d Deps) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.handleFleetStream(w, r)
	}))
}

type fleetEv struct {
	name string
	data map[string]any
}

// fleetEvents lee N eventos SSE del stream y corta la conexión.
func fleetEvents(t *testing.T, url string, want int) []fleetEv {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	out := []fleetEv{}
	for len(out) < want {
		ev, data := readEvent(t, sc)
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("data no JSON: %v", err)
		}
		out = append(out, fleetEv{ev, m})
	}
	return out
}

func TestFleetStreamSinHub503(t *testing.T) {
	d, _, done := testDeps(t)
	defer done()
	srv := fleetServer(t, d)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, quiere 503", resp.StatusCode)
	}
}

func TestFleetStreamSnapshotYReplay(t *testing.T) {
	d, _, done := testDeps(t)
	defer done()
	d.Hub = hub.New()
	srv := fleetServer(t, d)
	defer srv.Close()

	// cursor 0: snapshots de los tópicos pedidos y hello final con resync.
	got := fleetEvents(t, srv.URL+"?topics=status,deployments", 3)
	topics := map[string]bool{}
	for _, e := range got[:2] {
		if e.name != "snapshot" {
			t.Fatalf("quiere snapshot, got %q", e.name)
		}
		topics[e.data["topic"].(string)] = true
	}
	if !topics["status"] || !topics["deployments"] {
		t.Fatalf("topics = %v", topics)
	}
	if got[2].name != "hello" || got[2].data["resync"] != true {
		t.Fatalf("hello = %+v", got[2])
	}
	seq := int64(got[2].data["seq"].(float64))
	// El snapshot de status trae la app del fixture.
	found := false
	for _, e := range got[:2] {
		if e.data["topic"] == "status" {
			if s, ok := e.data["data"].(map[string]any); ok {
				if apps, ok := s["apps"].([]any); ok && len(apps) == 1 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("snapshot status sin apps: %+v", got[:2])
	}

	// Publica en vivo y reconecta con cursor intermedio: replay sin snapshots.
	d.Hub.Publish(hub.TopicDeployments, "deployments", []string{"d1"})
	mid := d.Hub.Seq()
	if mid != seq+1 {
		t.Fatalf("seq publish = %d, quiere %d", mid, seq+1)
	}
	d.Hub.Publish(hub.TopicDeployments, "deployments", []string{"d2"})
	got = fleetEvents(t, srv.URL+"?cursor="+strconv.FormatInt(mid, 10), 2)
	if got[0].name != "deployments" {
		t.Fatalf("replay = %q, quiere deployments", got[0].name)
	}
	if int64(got[0].data["seq"].(float64)) != mid+1 {
		t.Fatalf("seq replay = %v, quiere %d", got[0].data["seq"], mid+1)
	}
	if got[1].name != "hello" || got[1].data["resync"] != false {
		t.Fatalf("hello replay = %+v", got[1])
	}
}
