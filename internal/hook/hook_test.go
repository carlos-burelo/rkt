package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

func sign(t *testing.T, body, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func liveEmpty(t *testing.T) *config.Live {
	t.Helper()
	f, err := os.CreateTemp("", "rkt-hook-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("routes:\n  - hosts: [a.test]\n    target: http://127.0.0.1:9\napps:\n  - name: web\n    command: x\n    dir: '" + t.TempDir() + "'\n    deploy:\n      repo: https://github.com/o/r.git\n      ref: main\n      auto: true\n")
	f.Close()
	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return config.NewLive(cfg)
}

func TestPushMatchDeploys(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mu sync.Mutex
	var deployed []string
	h := NewHandler(liveEmpty(t), "s3cr3t", func(name string) {
		mu.Lock()
		deployed = append(deployed, name)
		mu.Unlock()
	}, log)

	body := `{"ref":"refs/heads/main","deleted":false,"repository":{"full_name":"o/r"}}`
	req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(t, body, "s3cr3t"))
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("code=%d", rec.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(deployed)
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("deployFn no corrió, deployed=%v", deployed)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if deployed[0] != "web" {
		t.Fatalf("deployed=%v", deployed)
	}
}

func TestPushIgnored(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	calls := 0
	h := NewHandler(liveEmpty(t), "s3cr3t", func(string) { calls++ }, log)
	signed := func(body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", sign(t, body, "s3cr3t"))
		req.Header.Set("X-GitHub-Event", "push")
		return req
	}

	cases := map[string]string{
		"otra rama":  `{"ref":"refs/heads/dev","deleted":false,"repository":{"full_name":"o/r"}}`,
		"otro repo":  `{"ref":"refs/heads/main","deleted":false,"repository":{"full_name":"o/otro"}}`,
		"borrado":    `{"ref":"refs/heads/main","deleted":true,"repository":{"full_name":"o/r"}}`,
		"json roto":  `{no-json`,
		"no es push": "",
	}
	for name, body := range cases {
		req := signed(body)
		if name == "no es push" {
			req.Header.Set("X-GitHub-Event", "ping")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s: code=%d", name, rec.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("nada debió desplegar, calls=%d", calls)
	}
}

func TestAuth(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(liveEmpty(t), "s3cr3t", func(string) {}, log)
	body := `{"ref":"refs/heads/main","deleted":false,"repository":{"full_name":"o/r"}}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin firma: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/_rkt/hook/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sign(t, body, "otra"))
	req.Header.Set("X-GitHub-Event", "push")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("firma mala: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_rkt/hook/github", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: code=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/otra", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("otra ruta: code=%d", rec.Code)
	}
}
