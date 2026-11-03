package discover

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
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

	"rkt-proxy/internal/github"
	"rkt-proxy/internal/store"
)

func testKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "app.pem")
	os.WriteFile(p, pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}), 0o600)
	return p
}

// fakeAPI sirve access_tokens + installations/repositories + contents.
func fakeAPI(t *testing.T, reposJSON string, files map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"token":      "tok",
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
		case strings.HasSuffix(r.URL.Path, "/installation/repositories"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"repositories": %s}`, reposJSON)
		case strings.HasSuffix(r.URL.Path, "/app/installations"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"id":999,"account":{"login":"o"}}]`)
		case strings.Contains(r.URL.Path, "/contents/"):
			name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if body, ok := files[name]; ok {
				json.NewEncoder(w).Encode(map[string]string{
					"encoding": "base64",
					"content":  base64.StdEncoding.EncodeToString([]byte(body)),
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testDeps(t *testing.T, srv *httptest.Server, pem string) (Deps, *store.DB, *[]string) {
	t.Helper()
	gh, err := github.New(1, pem, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetSetting("base_domain", "coatl.in")
	db.SetSetting("node_bin", os.Args[0]) // binario existente cualquiera
	var deployed []string
	return Deps{
		GH: gh, DB: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DirBase: t.TempDir(),
		DeployFn: func(name, actor string) (string, error) {
			deployed = append(deployed, name)
			return "rev1", nil
		},
	}, db, &deployed
}

func TestDiscoverNext(t *testing.T) {
	pem := testKey(t)
	srv := fakeAPI(t,
		`[{"full_name":"o/mi-app","default_branch":"main"}]`,
		map[string]string{"package.json": `{"dependencies":{"next":"14.0.0"}}`})
	defer srv.Close()
	d, db, deployed := testDeps(t, srv, pem)

	if _, err := Run(d); err != nil {
		t.Fatalf("run: %v", err)
	}
	fleet, _ := db.LoadFleet()
	if len(fleet.Apps) != 1 || fleet.Apps[0].Name != "mi-app" {
		t.Fatalf("app no registrada: %+v", fleet.Apps)
	}
	a := fleet.Apps[0]
	if a.Command == "" || a.Dir == "" || a.Deploy == nil || !a.Deploy.Auto {
		t.Fatalf("app incompleta: %+v", a)
	}
	if len(fleet.Routes) != 1 || fleet.Routes[0].Hosts[0] != "mi-app.coatl.in" {
		t.Fatalf("route mal: %+v", fleet.Routes)
	}
	if len(*deployed) != 1 || (*deployed)[0] != "mi-app" {
		t.Fatalf("deploy no disparado: %v", *deployed)
	}
	evs, err := db.ListEvents(10, "discover", "")
	if err != nil || len(evs) != 1 || evs[0].App != "mi-app" {
		t.Fatalf("evento discover mal: %+v %v", evs, err)
	}

	// idempotente: segunda vuelta no duplica ni redespliega
	if _, err := Run(d); err != nil {
		t.Fatalf("run2: %v", err)
	}
	fleet, _ = db.LoadFleet()
	if len(fleet.Apps) != 1 || len(fleet.Routes) != 1 || len(*deployed) != 1 {
		t.Fatalf("no idempotente: apps=%d routes=%d deployed=%v",
			len(fleet.Apps), len(fleet.Routes), *deployed)
	}
}

func TestDiscoverUnknownStack(t *testing.T) {
	pem := testKey(t)
	srv := fakeAPI(t,
		`[{"full_name":"o/rara","default_branch":"main"}]`,
		map[string]string{})
	defer srv.Close()
	d, db, deployed := testDeps(t, srv, pem)

	if _, err := Run(d); err != nil {
		t.Fatalf("run: %v", err)
	}
	fleet, _ := db.LoadFleet()
	if len(fleet.Apps) != 0 || len(*deployed) != 0 {
		t.Fatalf("stack raro no debe registrar: %+v %v", fleet.Apps, *deployed)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Mi-App": "mi-app", "a_b_c": "a-b-c", "x.y!z": "x-y-z", "ABC": "abc",
	} {
		if got := slug(in); got != want {
			t.Fatalf("slug(%q)=%q quiere %q", in, got, want)
		}
	}
	if slug("!!!") != "" {
		t.Fatal("slug vacío esperado")
	}
}

func TestFreePortSkipsUsed(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// ocupa un puerto real del pool
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	used := l.Addr().String()
	var port int
	fmt.Sscanf(used[strings.LastIndex(used, ":")+1:], "%d", &port)
	db.SetSetting("port_pool", fmt.Sprintf("%d-%d", port, port+1))
	// registra el ocupado en routes para probar ambas vías
	db.AddRoute([]string{"x.test"}, fmt.Sprintf("http://127.0.0.1:%d", port))

	got, err := freePort(db)
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	if got == port {
		t.Fatalf("debió saltar el ocupado %d", port)
	}
	if got != port+1 {
		t.Fatalf("debió dar %d, got %d", port+1, got)
	}
}
