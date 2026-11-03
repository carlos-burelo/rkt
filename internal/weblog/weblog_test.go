package weblog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `127.0.0.1:1 - - [03/Nov/2026:10:00:00 -0600] "GET /a?x=1 HTTP/1.1" 200 100 "-" "Test/1" host=a.test dur=1.5ms
127.0.0.1:2 - - [03/Nov/2026:10:01:00 -0600] "POST /b HTTP/1.1" 500 200 "https://x.test/" "Test/1" host=b.test dur=150ms
127.0.0.1:3 - - [03/Nov/2026:10:02:00 -0600] "GET /a HTTP/1.1" 404 50 "-" "Test/1" host=a.test dur=300us
línea rota sin sentido
127.0.0.1:4 - - [03/Nov/2026:10:03:00 -0600] "GET /a HTTP/1.1" 200 - "-" "Test/1" host=a.test dur=2ms
`

func TestParseLineHostEntreComillas(t *testing.T) {
	// el access log real escribe host=%q; el parser debe quitar comillas
	e, ok := ParseLine(`127.0.0.1:9 - - [03/Nov/2026:10:00:00 -0600] "GET / HTTP/2.0" 200 10 "-" "ua" host="sicesa.localhost" dur=2ms`)
	if !ok {
		t.Fatal("debe parsear")
	}
	if e.Host != "sicesa.localhost" {
		t.Fatalf("host = %q", e.Host)
	}
}

func TestParseLine(t *testing.T) {
	e, ok := ParseLine(`127.0.0.1:9 - - [03/Nov/2026:10:00:00 -0600] "GET /a?x=1 HTTP/1.1" 200 100 "-" "Test/1" host=a.test dur=1.5ms`)
	if !ok {
		t.Fatal("debe parsear")
	}
	if e.Method != "GET" || e.Path != "/a" || e.Status != 200 || e.Bytes != 100 {
		t.Fatalf("campos mal: %+v", e)
	}
	if e.Host != "a.test" || e.Referer != "-" || e.UA != "Test/1" {
		t.Fatalf("extras mal: %+v", e)
	}
	if e.Dur <= 0 {
		t.Fatalf("dur mal: %v", e.Dur)
	}
	if _, ok := ParseLine("basura"); ok {
		t.Fatal("basura debe fallar")
	}
	if _, ok := ParseLine(""); ok {
		t.Fatal("vacía debe fallar")
	}
}

func TestParseFileYSummary(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "access.log"), []byte(sample), 0o644)
	es := ParseFile(filepath.Join(dir, "access.log"))
	if len(es) != 4 {
		t.Fatalf("quiere 4 (1 rota fuera), got %d", len(es))
	}
	s := Summarize(es)
	if s.Total != 4 || s.Bytes != 350 {
		t.Fatalf("total/bytes mal: %+v", s)
	}
	if s.ByClass["2xx"] != 2 || s.ByClass["4xx"] != 1 || s.ByClass["5xx"] != 1 {
		t.Fatalf("clases mal: %+v", s.ByClass)
	}
	if len(s.ByPath) == 0 || s.ByPath[0].K != "/a" || s.ByPath[0].V != 3 {
		t.Fatalf("top path mal: %+v", s.ByPath)
	}
	if s.P50 == "" || s.P95 == "" {
		t.Fatalf("percentiles vacíos: %+v", s)
	}
	if strings.Contains(s.ByRef[0].K, "x.test") == false && len(s.ByRef) != 1 {
		t.Fatalf("referers mal: %+v", s.ByRef)
	}
	// sin .1 no falla
	if es := ParseFile(filepath.Join(dir, "noexiste.log")); len(es) != 0 {
		t.Fatalf("inexistente debe dar vacío, got %d", len(es))
	}
}

// FuzzParseLine: ninguna línea del mundo real puede tumbar el parser.
func FuzzParseLine(f *testing.F) {
	for _, seed := range strings.Split(sample, "\n") {
		f.Add(seed)
	}
	f.Add("")
	f.Add("[")
	f.Add(`1.2.3.4 - - [03/Nov/2026:10:00:00 -0600] "GET / HTTP/1.1" 200 1 "-" "u" host="a.test" dur=1ms`)
	f.Fuzz(func(t *testing.T, line string) {
		e, ok := ParseLine(line)
		if !ok {
			return
		}
		if e.Method == "" {
			t.Fatalf("ok sin método: %q", line)
		}
	})
}
