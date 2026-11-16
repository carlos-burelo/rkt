package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"rkt/internal/store"
)

// mcpAuth: loopback entra sin token; remoto exige API key válida.
func TestMcpAuthLoopback(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateUser("adm", "secreto-largo-123", store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	tok, err := db.CreateToken("adm", store.RoleAdmin, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	h := mcpAuth(db, next)
	call := func(remoteAddr, token string) int {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		req.RemoteAddr = remoteAddr
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := call("127.0.0.1:1", ""); code != http.StatusOK {
		t.Fatalf("loopback sin token: %d", code)
	}
	if code := call("[::1]:1", ""); code != http.StatusOK {
		t.Fatalf("loopback ::1 sin token: %d", code)
	}
	if code := call("203.0.113.7:1", ""); code != http.StatusUnauthorized {
		t.Fatalf("remoto sin token: %d", code)
	}
	if code := call("203.0.113.7:1", "mala"); code != http.StatusUnauthorized {
		t.Fatalf("remoto token malo: %d", code)
	}
	if code := call("203.0.113.7:1", tok); code != http.StatusOK {
		t.Fatalf("remoto token válido: %d", code)
	}
}
