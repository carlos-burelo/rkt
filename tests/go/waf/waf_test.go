package waf

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"rkt-proxy/internal/config"
)

func liveWith(t *testing.T, deny []string) *config.Live {
	t.Helper()
	cfg := config.Default()
	cfg.Routes = []config.Route{{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"}}
	if deny != nil {
		cfg.DenyPaths = deny
	}
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	return config.NewLive(cfg)
}

func TestDeny(t *testing.T) {
	resetBans()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, []string{"/.env", "/php-cgi*", "/php-cgi/*", "/.git/*"})

	for _, p := range []string{"/.env", "/php-cgi", "/php-cgi/php-cgi.exe", "/.git/config"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, quiere 403", p, rec.Code)
		}
	}
	for _, p := range []string{"/", "/login", "/env", "/phpinfo", "/api/health", "/_next/static/x.js"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s: got %d, quiere paso", p, rec.Code)
		}
	}
}

func TestDenyDepthAndHeaders(t *testing.T) {
	resetBans()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, nil) // defaults

	// Profundidad: el basename manda.
	for _, p := range []string{
		"/.env", "/.env.production", "/api/.env",
		"/x.php", "/api/x.php", "/wp-admin/setup-config.php",
		"/backup.sql", "/db/dump.sql", "/config.xml", "/database.sqlite",
		"/.ssh/id_rsa", "/_vti_pvt/service.pwd", "/docker-compose.yml",
		"/.git/config", "/actuator/health", "/wp-login.php",
	} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, quiere 403", p, rec.Code)
		}
	}
	// Cabecera interna de Next desde fuera = ataque (CVE-2025-29927).
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Middleware-Subrequest", "middleware")
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("header interno: got %d, quiere 403", rec.Code)
	}
	// Cabeceras normales pasan.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc")
	rec = httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("header normal: got %d, quiere paso", rec.Code)
	}
}

func TestDefaultsDenyCommon(t *testing.T) {
	resetBans()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, nil) // defaults
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest("GET", "/.env", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("default debe denegar /.env, got %d", rec.Code)
	}
}

func TestValidPattern(t *testing.T) {
	if !ValidPattern("/a/*.php") || ValidPattern("/a/[") {
		t.Fatal("validación de glob mal")
	}
}

func resetBans() {
	banMu.Lock()
	defer banMu.Unlock()
	bans = map[string]*banState{}
	ringMu.Lock()
	defer ringMu.Unlock()
	ring = nil
}

func TestAutoBanUmbral(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	resetBans()
	live := liveWith(t, []string{"/.env"})
	now := time.Now()

	// 19 golpes: aún pasa el resto (aquí todo es 403 por el path).
	for i := 0; i < banThreshold-1; i++ {
		req := httptest.NewRequest("GET", "/.env", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("golpe %d: got %d", i, rec.Code)
		}
	}
	// el golpe 20 banea: hasta una ruta limpia da 403.
	req := httptest.NewRequest("GET", "/.env", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	Middleware(live, next, log).ServeHTTP(httptest.NewRecorder(), req)
	reqOK := httptest.NewRequest("GET", "/", nil)
	reqOK.RemoteAddr = "9.9.9.9:1234"
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, reqOK)
	if rec.Code != http.StatusForbidden {
		t.Fatal("baneado debe dar 403 hasta en ruta limpia")
	}
	// otra IP no afectada.
	reqOther := httptest.NewRequest("GET", "/", nil)
	reqOther.RemoteAddr = "9.9.9.8:1234"
	rec = httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, reqOther)
	if rec.Code != http.StatusTeapot {
		t.Fatal("otra IP debe pasar")
	}
	_ = now
}

func TestAutoBanExpiraYNoSeExtiende(t *testing.T) {
	resetBans()
	base := time.Now()
	for i := 0; i < banThreshold; i++ {
		hit("1.1.1.1", base.Add(time.Duration(i)*time.Second))
	}
	if !bannedSince("1.1.1.1", base.Add(time.Minute)) {
		t.Fatal("debió banear")
	}
	// golpes durante el ban no extienden el TTL.
	hit("1.1.1.1", base.Add(5*time.Minute))
	if bannedSince("1.1.1.1", base.Add(banTTL).Add(-time.Second)) != true {
		t.Fatal("TTL fijo: debe seguir baneado hasta el final")
	}
	// el ban arranca en el golpe 20 (t=19s), no en t=0.
	if bannedSince("1.1.1.1", base.Add(banTTL).Add(20*time.Second)) {
		t.Fatal("TTL vencido: debe expirar")
	}
	// hits viejos fuera de ventana no cuentan.
	resetBans()
	hit("2.2.2.2", base)
	if bannedSince("2.2.2.2", base.Add(banWindow).Add(time.Minute)) {
		t.Fatal("un golpe viejo no debe banear")
	}
}

func TestBannedYUnban(t *testing.T) {
	resetBans()
	base := time.Now()
	for i := 0; i < banThreshold; i++ {
		hit("3.3.3.3", base.Add(time.Duration(i)*time.Second))
	}
	got := Banned()
	if len(got) != 1 || got[0].IP != "3.3.3.3" || got[0].Hits < banThreshold || got[0].Until == "" {
		t.Fatalf("banned = %+v", got)
	}
	if !Unban("3.3.3.3") {
		t.Fatal("unban debe dar true")
	}
	if Unban("3.3.3.3") {
		t.Fatal("doble unban debe dar false")
	}
	if len(Banned()) != 0 {
		t.Fatal("tras unban no debe haber baneos")
	}
}

func TestDenyMethods(t *testing.T) {
	resetBans()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, nil) // defaults traen TRACE/TRACK/CONNECT

	for _, m := range []string{"TRACE", "TRACK", "CONNECT"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest(m, "/", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, quiere 403", m, rec.Code)
		}
	}
	for _, m := range []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"} {
		rec := httptest.NewRecorder()
		Middleware(live, next, log).ServeHTTP(rec, httptest.NewRequest(m, "/", nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s: got %d, quiere paso", m, rec.Code)
		}
	}
}

func TestDenyEscapado(t *testing.T) {
	resetBans()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	live := liveWith(t, []string{"/.env", "/s%20dir/*"})

	// Patrón con escape: el path decodificado no matchea, el escapado sí.
	u, err := url.Parse("http://a.test/s%20dir/x")
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{Method: "GET", URL: u, Host: "a.test", Header: http.Header{}}
	rec := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("escapado: got %d, quiere 403", rec.Code)
	}
	// Sin escape no hay falso positivo: otra carpeta con espacio pasa.
	u2, _ := url.Parse("http://a.test/otra%20dir/x")
	req2 := &http.Request{Method: "GET", URL: u2, Host: "a.test", Header: http.Header{}}
	rec2 := httptest.NewRecorder()
	Middleware(live, next, log).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTeapot {
		t.Fatalf("otra carpeta: got %d, quiere paso", rec2.Code)
	}
}

func TestRecentRing(t *testing.T) {
	resetBans()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	ringMu.Lock()
	ring = nil
	ringMu.Unlock()

	live := liveWith(t, []string{"/.env"})
	req := httptest.NewRequest("TRACE", "/.env", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	req.Host = "a.test"
	Middleware(live, next, log).ServeHTTP(httptest.NewRecorder(), req)

	recent := Recent()
	if len(recent) != 1 {
		t.Fatalf("bloqueos = %d, quiere 1", len(recent))
	}
	b := recent[0]
	if b.IP != "9.9.9.9" || b.Host != "a.test" || b.Method != "TRACE" || b.Path != "/.env" || b.Rule == "" || b.At == "" {
		t.Fatalf("bloqueo mal registrado: %+v", b)
	}
	// el método manda sobre el path en el motivo
	if !strings.Contains(b.Rule, "TRACE") {
		t.Fatalf("motivo = %q, quiere el método", b.Rule)
	}

	// tope del anillo: solo sobreviven los últimos 50
	ringMu.Lock()
	ring = nil
	for i := 0; i < 60; i++ {
		ring = append(ring, Block{Path: fmt.Sprintf("/%d", i)})
	}
	ringMu.Unlock()
	req = httptest.NewRequest("GET", "/.env", nil)
	Middleware(live, next, log).ServeHTTP(httptest.NewRecorder(), req)
	if got := len(Recent()); got != maxBlocks {
		t.Fatalf("anillo = %d, quiere %d", got, maxBlocks)
	}
}
