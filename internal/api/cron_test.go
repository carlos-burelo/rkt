package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/cronjobs"
	"rkt-proxy/internal/store"
)

func TestCronAPI(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.Cron = cronjobs.New(d.Live, db, d.LogsDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cron/w/eco",
		strings.NewReader(`{"schedule":"* * * * *","command":"echo cron-api","timeout":"1m"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/cron", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"job":"eco"`) || !strings.Contains(rec.Body.String(), `"next"`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/cron/w/eco/run", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := db.ListCronRuns("w", "eco", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) > 0 && runs[0].Status != "running" {
			if runs[0].Status != "ok" {
				t.Fatalf("run = %+v", runs[0])
			}
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/cron/w/eco/runs", tok))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
				t.Fatalf("runs: %d %s", rec.Code, rec.Body.String())
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("cron no terminó")
}

func TestCronAPIHTTP(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cron/w/ping",
		strings.NewReader(`{"schedule":"* * * * *","type":"http","path":"/health","method":"POST","timeout":"1m"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put http: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/cron", tok))
	body := rec.Body.String()
	for _, want := range []string{`"type":"http"`, `"path":"/health"`, `"method":"POST"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("list http sin %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"command":"`) {
		t.Fatalf("tarea http no debe exponer command: %s", body)
	}
}
