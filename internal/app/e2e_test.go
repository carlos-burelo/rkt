package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/cfgfile"
	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func waitHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		resp, err := http.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no respondió %s: %v", url, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestE2EProxyWAFBodyYAccessLog arranca el servicio real (DB temporal,
// servidores HTTP/healthz reales) y comprueba el camino completo:
// proxy -> backend, WAF, tope de body, access log y auth fail-closed.
func TestE2EProxyWAFBodyYAccessLog(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("backend:" + r.URL.Path))
	}))
	defer backend.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	httpAddr := freeAddr(t)
	healthAddr := freeAddr(t)
	yaml := fmt.Sprintf(`{"listen_http":%q,"listen_https":"","redirect_http_to_https":false,"rate_limit":{"enabled":false},"healthz":{"enabled":true,"listen":%q},"max_body_mb":1,"routes":[{"hosts":["e2e.test"],"target":%q}]}`,
		httpAddr, healthAddr, backend.URL)
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	seed, _, err := config.ImportJSON([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	seedDB, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfgfile.SaveFleet(seedDB, seed); err != nil {
		t.Fatal(err)
	}
	if err := cfgfile.SaveInfra(seedDB, seed); err != nil {
		t.Fatal(err)
	}
	_ = seedDB.Close()

	p := New(cfgPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go p.Run()
	defer func() { _ = p.Stop(nil) }()
	waitHTTP(t, "http://"+healthAddr+"/healthz", 10*time.Second)
	// healthz puede arrancar antes que el listener público: espera ambos.
	waitHTTP(t, "http://"+httpAddr+"/", 10*time.Second)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, httpAddr)
			},
		},
	}
	do := func(method, path string, body io.Reader) (*http.Response, string, error) {
		req, err := http.NewRequest(method, "http://e2e.test"+path, body)
		if err != nil {
			return nil, "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b), nil
	}

	resp, body, err := do(http.MethodGet, "/hola?x=1", nil)
	if err != nil {
		t.Fatalf("proxy: %v", err)
	}
	if resp.StatusCode != http.StatusOK || body != "backend:/hola" {
		t.Fatalf("proxy = %d %q", resp.StatusCode, body)
	}

	// WAF: deny_paths por defecto incluye /.env
	resp, _, err = do(http.MethodGet, "/.env", nil)
	if err != nil {
		t.Fatalf("waf: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf(".env = %d, quiere 403", resp.StatusCode)
	}

	// tope de body sin Content-Length (chunked) => 413, no 502
	big := bytes.Repeat([]byte("a"), 2<<20)
	resp, _, err = do(http.MethodPost, "/subir", io.NopCloser(bytes.NewReader(big)))
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("body grande = %d, quiere 413", resp.StatusCode)
	}

	// access log con el host real
	logPath := logx.AccessPath(cfgPath)
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, _ := os.ReadFile(logPath)
		if strings.Contains(string(data), `host="e2e.test"`) && strings.Contains(string(data), "GET /hola") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("access log sin la petición: %q", string(data))
		}
		time.Sleep(50 * time.Millisecond)
	}

	// API fail-closed sin sesión
	resp, err = http.Get("http://" + healthAddr + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/api/v1/status = %d, quiere 401", resp.StatusCode)
	}
}
