package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rkt/internal/logx"
	"rkt/internal/store"
)

func traceOut(t *testing.T, out string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("JSON inválido %q: %v", out, err)
	}
	return v
}

func chainServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fin"))
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/long", func(w http.ResponseWriter, r *http.Request) {
		n := 0
		fmt.Sscanf(r.URL.Query().Get("n"), "%d", &n)
		if n >= 10 {
			w.Write([]byte("fin"))
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/long?n=%d", n+1), http.StatusFound)
	})
	mux.HandleFunc("/noloc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://[::1")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/ftp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "ftp://x/y")
		w.WriteHeader(http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestTraceRedirectsOK(t *testing.T) {
	base := chainServer(t)
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	out, err := s.doTraceRedirects(traceArg{URL: base + "/a"})
	if err != nil {
		t.Fatal(err)
	}
	v := traceOut(t, out)
	if v["verdict"] != "ok" {
		t.Fatalf("verdict = %v, out = %s", v["verdict"], out)
	}
	if v["final_url"] != base+"/b" {
		t.Fatalf("final = %v", v["final_url"])
	}
	if v["hops_count"] != 2.0 {
		t.Fatalf("hops = %v", v["hops_count"])
	}
}

func TestTraceRedirectsLoop(t *testing.T) {
	base := chainServer(t)
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	out, err := s.doTraceRedirects(traceArg{URL: base + "/loop"})
	if err != nil {
		t.Fatal(err)
	}
	if traceOut(t, out)["verdict"] != "redirect_loop" {
		t.Fatalf("debió detectar loop: %s", out)
	}
}

func TestTraceRedirectsTooMany(t *testing.T) {
	base := chainServer(t)
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	out, err := s.doTraceRedirects(traceArg{URL: base + "/long", MaxHops: 3})
	if err != nil {
		t.Fatal(err)
	}
	if traceOut(t, out)["verdict"] != "too_many_redirects" {
		t.Fatalf("debió agotar saltos: %s", out)
	}
}

func TestTraceRedirectsLocationInvalida(t *testing.T) {
	base := chainServer(t)
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	// Go rechaza Location inparseable en el transporte: llega como error de red.
	out, err := s.doTraceRedirects(traceArg{URL: base + "/noloc"})
	if err != nil {
		t.Fatal(err)
	}
	if traceOut(t, out)["verdict"] != "network_error" {
		t.Fatalf("debió ser error de red: %s", out)
	}
	out, err = s.doTraceRedirects(traceArg{URL: base + "/ftp"})
	if err != nil {
		t.Fatal(err)
	}
	if traceOut(t, out)["verdict"] != "non_http_target" {
		t.Fatalf("debió detenerse en esquema no http: %s", out)
	}
}

func TestTraceRedirectsMalUso(t *testing.T) {
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := s.doTraceRedirects(traceArg{URL: "nota-url"}); err == nil {
		t.Fatal("URL mala debió fallar")
	}
	if _, err := s.doTraceRedirects(traceArg{URL: "ftp://x/y"}); err == nil {
		t.Fatal("esquema no http debió fallar")
	}
	if _, err := s.doTraceRedirects(traceArg{URL: "http://127.0.0.1:9/", Method: "POST"}); err == nil {
		t.Fatal("method POST debió fallar")
	}
}

func TestManifestRedirectRules(t *testing.T) {
	cases := []struct {
		url string
		bad bool
		why string
	}{
		{"http://localhost:18080/_rkt/setup?state=s", true, "localhost con puerto"},
		{"http://127.0.0.1:18080/_rkt/setup?state=s", false, "127 con puerto"},
		{"http://localhost/_rkt/setup", false, "localhost sin puerto"},
		{"https://example.com/_rkt/setup?state=s", false, "dominio público"},
		{"nota-url", true, "no URL"},
		{"ftp://x/y", true, "esquema"},
		{"http:///sin-host", true, "sin host"},
	}
	for _, c := range cases {
		ps := checkManifestRedirectURL(c.url)
		if c.bad && len(ps) == 0 {
			t.Fatalf("%s (%s): debió dar problema", c.url, c.why)
		}
		if !c.bad && len(ps) > 0 {
			t.Fatalf("%s (%s): problema inesperado %v", c.url, c.why, ps)
		}
	}
	if !strings.Contains(strings.Join(checkManifestRedirectURL("http://localhost:1/x"), " "), "127.0.0.1") {
		t.Fatal("el problema debe sugerir 127.0.0.1")
	}
}

func setBaseDomain(t *testing.T, s *Server, base string) {
	t.Helper()
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetSetting(store.KeyBaseDomain, base); err != nil {
		t.Fatal(err)
	}
}

func TestGithubSetupCheckLocal(t *testing.T) {
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	out, err := s.doGithubSetupCheck()
	if err != nil {
		t.Fatal(err)
	}
	v := traceOut(t, out)
	if v["verdict"] != "ok" {
		t.Fatalf("local sin base debió ser ok: %s", out)
	}
	if v["local"] != true {
		t.Fatalf("local = %v", v["local"])
	}
	if !strings.HasPrefix(v["redirect_url"].(string), "http://127.0.0.1") {
		t.Fatalf("redirect_url = %v", v["redirect_url"])
	}
}

func TestGithubSetupCheckBase(t *testing.T) {
	s := testServer(t, func(w http.ResponseWriter, r *http.Request) {})
	setBaseDomain(t, s, "example.com")
	out, err := s.doGithubSetupCheck()
	if err != nil {
		t.Fatal(err)
	}
	v := traceOut(t, out)
	if v["verdict"] != "ok" || v["local"] != false {
		t.Fatalf("con base debió ser ok no-local: %s", out)
	}
	if v["redirect_url"] != "https://example.com/_rkt/setup" {
		t.Fatalf("redirect_url = %v", v["redirect_url"])
	}
}
