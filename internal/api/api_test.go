package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/health"
	"rkt-proxy/internal/store"
)

func testDeps(t *testing.T) (Deps, *store.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(filepath.Join(logsDir, "apps"), 0o755)
	os.WriteFile(filepath.Join(logsDir, "rkt.log"), []byte("l1\nl2\nl3\n"), 0o644)
	cfg := &config.Config{Routes: []config.Route{
		{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"},
	}}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	d := Deps{
		DB: db, Live: config.NewLive(cfg), Version: "test",
		LogsDir: logsDir,
		Apps:    func() []health.AppStatus { return []health.AppStatus{{Name: "w", Running: true}} },
	}
	return d, db, func() { db.Close() }
}

func login(t *testing.T, h http.Handler, user, pass string) string {
	t.Helper()
	body := `{"user":` + strconv_quote(user) + `,"password":` + strconv_quote(pass) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("login sin token: %s", rec.Body.String())
	}
	return out.Token
}

func strconv_quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func authed(method, path, tok string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

func TestSinUsuariosNadaOpera(t *testing.T) {
	d, _, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	// login imposible sin usuarios
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"user":"a","password":"password-larga-1"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("quiere 401, got %d", rec.Code)
	}
	// status exige sesión
	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("quiere 401, got %d", rec.Code)
	}
}

func TestLoginStatusLogs(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	if err := db.CreateUser("op", "password-larga-1", store.RoleOperator); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(d)
	tok := login(t, h, "op", "password-larga-1")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var st struct {
		Version string             `json:"version"`
		Routes  int                `json:"routes"`
		Apps    []health.AppStatus `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Version != "test" || st.Routes != 1 || len(st.Apps) != 1 {
		t.Fatalf("status mal: %+v", st)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/logs?n=2", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("logs: %d", rec.Code)
	}
	var lg struct {
		Log string `json:"log"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lg); err != nil {
		t.Fatal(err)
	}
	if lg.Log != "l2\nl3" {
		t.Fatalf("tail mal: %q", lg.Log)
	}

	// traversal bloqueado
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/logs?app=../x", tok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("traversal: quiere 400, got %d", rec.Code)
	}

	// logout invalida
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/auth/logout", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("logout: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", tok))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout: quiere 401, got %d", rec.Code)
	}
}

func TestLoginThrottle(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	if err := db.CreateUser("op", "password-larga-1", store.RoleOperator); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(d)
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"user":"op","password":"mal-mal-mal-mal"}`))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if i < 5 && rec.Code != http.StatusUnauthorized {
			t.Fatalf("intento %d: quiere 401, got %d", i, rec.Code)
		}
		if i == 5 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("intento 6: quiere 429, got %d", rec.Code)
		}
	}
}

func TestUsersSoloAdmin(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	if err := db.CreateUser("adm", "password-larga-1", store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateUser("vis", "password-larga-1", store.RoleViewer); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(d)
	admTok := login(t, h, "adm", "password-larga-1")
	visTok := login(t, h, "vis", "password-larga-1")

	// viewer no lista usuarios
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/users", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer: quiere 403, got %d", rec.Code)
	}
	// admin crea + lista + borra
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users",
		strings.NewReader(`{"name":"op2","password":"password-larga-2","role":"operator"}`))
	req.Header.Set("Authorization", "Bearer "+admTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/users", admTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var users []store.User
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil || len(users) != 3 {
		t.Fatalf("list mal: %+v %v", users, err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/users/op2", admTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("baja: %d", rec.Code)
	}
}
