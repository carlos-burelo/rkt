package proxy

// Las tres estrategias a nivel HTTP: dominio, prefijo de ruta (con
// recorte) y puerto del edge (listener real).

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func echoBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s", r.Host, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func proxyLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestProxyPathRecorta(t *testing.T) {
	back := echoBackend(t)
	target := strings.TrimPrefix(back.URL, "http://")
	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[
		{"hosts":["h.test"],"path":"/app","target":"http://%s"},
		{"hosts":["h.test"],"target":"http://%s"}
	]}`, target, target))
	h := NewHandler(live, proxyLog())

	get := func(host, path string) (int, string) {
		t.Helper()
		req := httptest.NewRequest("GET", "http://"+host+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		b, _ := io.ReadAll(rec.Result().Body)
		return rec.Code, string(b)
	}
	if code, body := get("h.test", "/app/hola"); code != 200 || body != target+"|/hola" {
		t.Fatalf("recorte: %d %q", code, body)
	}
	if code, body := get("h.test", "/app"); code != 200 || body != target+"|/" {
		t.Fatalf("prefijo exacto: %d %q", code, body)
	}
	// /app2 no es subruta de /app: cae en la ruta pelada del host.
	if code, body := get("h.test", "/app2"); code != 200 || body != target+"|/app2" {
		t.Fatalf("fallback host: %d %q", code, body)
	}
	if code, _ := get("otro.test", "/app/hola"); code != 404 {
		t.Fatalf("host desconocido: %d", code)
	}
}

func serveOn(t *testing.T, h http.Handler) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	go http.Serve(ln, h)
	// Espera a que acepte (Serve arranca en goroutine).
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("listener no arrancó")
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ln.Addr().String(), port
}

func TestProxyPortListenerReal(t *testing.T) {
	back := echoBackend(t)
	target := strings.TrimPrefix(back.URL, "http://")

	// Puerto reservado primero para declararlo en la ruta.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	var edgePort int
	fmt.Sscanf(portStr, "%d", &edgePort)

	live := liveFrom(t, fmt.Sprintf(`{"restart_grace":"0s","rate_limit":{"enabled":false},"routes":[
		{"hosts":["h.test"],"target":"http://%s"},
		{"port":%d,"target":"http://%s"}
	]}`, target, edgePort, target))
	h := NewHandler(live, proxyLog())

	edgeLn, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", edgePort))
	if err != nil {
		t.Skipf("puerto %d ocupado: %v", edgePort, err)
	}
	t.Cleanup(func() { edgeLn.Close() })
	go http.Serve(edgeLn, h)

	get := func(url, host string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", url, nil)
		if host != "" {
			req.Host = host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get %s: %v", url, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	// En el puerto extra manda el puerto aunque el host tenga ruta propia.
	if code, body := get(fmt.Sprintf("http://127.0.0.1:%d/cualquiera", edgePort), "h.test"); code != 200 || !strings.HasSuffix(body, "|/cualquiera") {
		t.Fatalf("port-based: %d %q", code, body)
	}
	// En otro puerto (httptest, aleatorio) la ruta de puerto no existe;
	// h.test sigue resolviendo por host.
	other := httptest.NewServer(h)
	defer other.Close()
	if code, _ := get(other.URL+"/x", "h.test"); code != 200 {
		t.Fatalf("host en puerto principal: %d", code)
	}
	if code, _ := get(other.URL+"/x", "nadie.test"); code != 404 {
		t.Fatalf("aislamiento: %d", code)
	}
}
