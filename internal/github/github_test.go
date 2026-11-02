package github

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "app.pem")
	os.WriteFile(p, pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key),
	}), 0o600)
	return key, p
}

// fakeAPI emula access_tokens + commits. Cuenta llamadas.
func fakeAPI(t *testing.T, pub *rsa.PublicKey, hits *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			// verifica JWT Bearer
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			parts := strings.Split(tok, ".")
			if len(parts) != 3 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			if rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig) != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"token":      "tok-secreto",
				"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			})
			return
		}
		if strings.Contains(r.URL.Path, "/commits/") {
			if r.Header.Get("Authorization") != "Bearer tok-secreto" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"sha": "abc123def456"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestTokenCached(t *testing.T) {
	key, path := testKey(t)
	var hits int64
	srv := fakeAPI(t, &key.PublicKey, &hits)
	defer srv.Close()

	m, err := New(123, path, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.TokenFor(456)
	if err != nil || tok != "tok-secreto" {
		t.Fatalf("token: %q %v", tok, err)
	}
	if _, err := m.TokenFor(456); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("segundo Token debe usar cache, hits=%d", hits)
	}
}

func TestTokenBadKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mala.pem")
	os.WriteFile(p, []byte("no-pem"), 0o600)
	if _, err := New(1, p, ""); err == nil {
		t.Fatal("clave mala debe fallar")
	}
	if _, err := New(1, filepath.Join(t.TempDir(), "falta.pem"), ""); err == nil {
		t.Fatal("clave faltante debe fallar")
	}
}

func TestCommitSHA(t *testing.T) {
	key, path := testKey(t)
	var hits int64
	srv := fakeAPI(t, &key.PublicKey, &hits)
	defer srv.Close()

	m, _ := New(123, path, srv.URL)
	sha, err := m.CommitSHA("o", "r", "main", "tok-secreto")
	if err != nil || sha != "abc123def456" {
		t.Fatalf("sha: %q %v", sha, err)
	}
	if _, err := m.CommitSHA("o", "r", "main", "mal-token"); err == nil {
		t.Fatal("token malo debe fallar")
	}
}

func TestParseRepo(t *testing.T) {
	for url, want := range map[string][2]string{
		"https://github.com/o/r.git": {"o", "r"},
		"https://github.com/o/r":     {"o", "r"},
		"https://github.com/o/r/":    {"o", "r"},
	} {
		o, r, ok := ParseRepo(url)
		if !ok || o != want[0] || r != want[1] {
			t.Fatalf("%s: got %q %q %v", url, o, r, ok)
		}
	}
	for _, bad := range []string{"", "notaurl", "https://github.com/solo"} {
		if _, _, ok := ParseRepo(bad); ok {
			t.Fatalf("%q debió fallar", bad)
		}
	}
}

func TestInstallations(t *testing.T) {
	_, path := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":7,"account":{"login":"miorg"}},{"id":9,"account":{"login":"yo"}}]`)
	}))
	defer srv.Close()

	m, err := New(123, path, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	list, err := m.Installations()
	if err != nil || len(list) != 2 || list[0].ID != 7 || list[1].Account.Login != "yo" {
		t.Fatalf("lista mal: %+v %v", list, err)
	}
}
