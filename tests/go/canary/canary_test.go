package canary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeExe(extra string) []byte {
	var head []byte
	switch runtime.GOOS {
	case "windows":
		head = []byte("MZ\x90\x00")
	case "linux":
		head = []byte{0x7f, 'E', 'L', 'F'}
	default:
		head = []byte{0xcf, 0xfa, 0xed, 0xfe}
	}
	return append(head, []byte(extra)...)
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestPutListGetRemove(t *testing.T) {
	st := Open(t.TempDir())
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	bin := fakeExe("uno")
	m, err := st.Put(bytes.NewReader(bin), "v0.7.0-1-gabc", sum(bin), "ci", now)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(m.ID) || m.SHA256 != sum(bin) || m.Size != int64(len(bin)) || m.Label != "v0.7.0-1-gabc" {
		t.Fatalf("meta: %+v", m)
	}
	got, path, err := st.Get(m.ID)
	if err != nil || got.ID != m.ID || !strings.HasSuffix(path, "rkt.exe") {
		t.Fatalf("get: %+v %q %v", got, path, err)
	}
	if l := st.List(); len(l) != 1 || l[0].ID != m.ID {
		t.Fatalf("list: %+v", l)
	}
	if err := st.Remove(m.ID); err != nil || len(st.List()) != 0 {
		t.Fatalf("remove: %v %v", err, st.List())
	}
}

func TestPutRechaza(t *testing.T) {
	st := Open(t.TempDir())
	now := time.Now()
	bin := fakeExe("x")
	cases := []struct {
		name, label, sha string
		body             []byte
	}{
		{"sha distinto", "ok", strings.Repeat("0", 64), bin},
		{"no ejecutable", "ok", "", []byte("#!/bin/sh\necho hola\n")},
		{"vacío", "ok", "", nil},
		{"label inválido", "../x", "", bin},
	}
	for _, c := range cases {
		if _, err := st.Put(bytes.NewReader(c.body), c.label, c.sha, "t", now); err == nil {
			t.Errorf("%s: debía fallar", c.name)
		}
	}
	if len(st.List()) != 0 {
		t.Fatal("un rechazo no debe dejar canaries")
	}
}

func TestPoda(t *testing.T) {
	st := Open(t.TempDir())
	base := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	var last Meta
	for i := 0; i < Keep+2; i++ {
		m, err := st.Put(bytes.NewReader(fakeExe(string(rune('a'+i)))), "", "", "t", base.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		last = m
	}
	l := st.List()
	if len(l) != Keep || l[0].ID != last.ID {
		t.Fatalf("poda: %d canaries, primero %s", len(l), l[0].ID)
	}
}

func TestIDInvalidoNoToca(t *testing.T) {
	st := Open(t.TempDir())
	for _, id := range []string{"", "..", "../x", "20260101-000000-zzzzzzzz", "x"} {
		if _, _, err := st.Get(id); err == nil {
			t.Errorf("%q debía fallar", id)
		}
		if err := st.Remove(id); err == nil {
			t.Errorf("remove %q debía fallar", id)
		}
	}
}

func TestFetchValidaAntesDeRed(t *testing.T) {
	st := Open(t.TempDir())
	ctx := context.Background()
	for _, c := range []struct{ url, sha string }{
		{"http://example.com/rkt.exe", strings.Repeat("a", 64)}, // no https
		{"https://example.com/rkt.exe", ""},                     // sin sha
		{"https://127.0.0.1/rkt.exe", strings.Repeat("a", 64)},  // loopback
		{"https://169.254.169.254/x", strings.Repeat("a", 64)},  // metadatos
		{"https://10.0.0.5/rkt.exe", strings.Repeat("a", 64)},   // privada
	} {
		if _, err := st.Fetch(ctx, c.url, "", c.sha, "t", time.Now()); err == nil {
			t.Errorf("%s debía fallar", c.url)
		}
	}
}
