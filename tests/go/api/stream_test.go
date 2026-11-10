package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// sseLine lee hasta encontrar una línea "event: X" y devuelve el data.
func readEvent(t *testing.T, sc *bufio.Scanner) (string, string) {
	t.Helper()
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			return event, strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatalf("stream cerrado sin data (último event=%q)", event)
	return "", ""
}

func TestStreamLogInitChunk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("hola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamLog(w, r, path, 10, nil)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	ev, data := readEvent(t, sc)
	if ev != "init" || !strings.Contains(data, "hola") {
		t.Fatalf("init = %q %q", ev, data)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("mundo\n")
	_ = f.Close()

	ev, data = readEvent(t, sc)
	if ev != "chunk" || !strings.Contains(data, "mundo") {
		t.Fatalf("chunk = %q %q", ev, data)
	}
}

func TestStreamLogEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var finished atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamLog(w, r, path, 10, func() bool { return finished.Load() })
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	if ev, _ := readEvent(t, sc); ev != "init" {
		t.Fatalf("primero init, got %q", ev)
	}
	finished.Store(true)
	ev, data := readEvent(t, sc)
	if ev != "end" || !strings.Contains(data, "done") {
		t.Fatalf("end = %q %q", ev, data)
	}
}

func TestStreamLogMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamLog(w, r, filepath.Join(t.TempDir(), "nope.log"), 5, nil)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	if ev, _ := readEvent(t, sc); ev != "init" {
		t.Fatalf("primero init, got %q", ev)
	}
	if ev, data := readEvent(t, sc); ev != "end" || !strings.Contains(data, "missing") {
		t.Fatalf("end = %q %q", ev, data)
	}
}
