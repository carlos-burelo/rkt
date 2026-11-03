package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	now := time.Now().Format("02/Jan/2006:15:04:05 -0700")
	access := "127.0.0.1:1 - - [" + now + "] \"GET /a HTTP/1.1\" 200 100 \"-\" \"T/1\" host=a.test dur=2ms\n" +
		"127.0.0.1:2 - - [" + now + "] \"GET /b HTTP/1.1\" 500 50 \"-\" \"T/1\" host=a.test dur=9ms\n"
	os.WriteFile(filepath.Join(logsDir, "access.log"), []byte(access), 0o644)
	cfg := &config.Config{
		Apps: []config.App{
			{Name: "w", Command: "c", Dir: dir, Env: map[string]string{"K": "v"}, RestartDelay: "5s"},
		},
		Routes: []config.Route{
			{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:9"},
		},
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	d := Deps{
		DB: db, Live: config.NewLive(cfg), Version: "test",
		LogsDir: logsDir,
		Apps:    func() []health.AppStatus { return []health.AppStatus{{Name: "w", Running: true}} },
		Restart: func(name string) bool { return name == "w" },
		Deploy: func(name, actor string) (string, error) {
			if name != "w" {
				return "", fmt.Errorf("no")
			}
			if actor == "" {
				return "", fmt.Errorf("sin actor")
			}
			return "rev1", nil
		},
		Rollback: func(name, actor string) error {
			if name != "w" {
				return fmt.Errorf("no")
			}
			return nil
		},
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

func loginAs(t *testing.T, h http.Handler, db *store.DB, name, role string) string {
	t.Helper()
	if err := db.CreateUser(name, "password-larga-1", role); err != nil {
		t.Fatal(err)
	}
	return login(t, h, name, "password-larga-1")
}

func TestAppsSinEnvYAcciones(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// lista sin env (secretos no salen)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("apps: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"K"`) {
		t.Fatalf("env filtrado: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"w"`) {
		t.Fatalf("falta app: %s", rec.Body.String())
	}

	// viewer no acciona
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/apps/w/restart", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer restart: quiere 403, got %d", rec.Code)
	}
	// operator sí
	for path, want := range map[string]int{
		"/api/v1/apps/w/restart":     http.StatusOK,
		"/api/v1/apps/w/deploy":      http.StatusOK,
		"/api/v1/apps/w/rollback":    http.StatusOK,
		"/api/v1/apps/nadie/restart": http.StatusNotFound,
	} {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, authed(http.MethodPost, path, opTok))
		if rec.Code != want {
			t.Fatalf("%s: quiere %d, got %d (%s)", path, want, rec.Code, rec.Body.String())
		}
	}
}

func TestRoutesCRUD(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/routes", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "a.test") {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	// alta duplicada rechaza
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes",
		strings.NewReader(`{"hosts":["a.test"],"target":"http://127.0.0.1:9"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("dup: quiere 400, got %d", rec.Code)
	}
	// alta ok + baja por host
	req = httptest.NewRequest(http.MethodPost, "/api/v1/routes",
		strings.NewReader(`{"hosts":["b.test"],"target":"http://127.0.0.1:10"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/routes?host=b.test", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("baja: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/routes?host=nadie.test", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("baja fantasma: quiere 404, got %d", rec.Code)
	}
}

func TestEnvCRUD(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// viewer no lee secretos
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w/env", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer env: quiere 403, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w/env", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"K":"v"`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	// set + unset
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/w/env",
		strings.NewReader(`{"key":"N","value":"1"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w/env", opTok))
	if !strings.Contains(rec.Body.String(), `"N":"1"`) {
		t.Fatalf("set no aplicado: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/apps/w/env?key=N", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("unset: %d", rec.Code)
	}
}

func TestSettingsRW(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/settings", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "base_domain") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings",
		strings.NewReader(`{"key":"base_domain","value":"ejemplo.com"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d", rec.Code)
	}
	// desconocido rechaza
	req = httptest.NewRequest(http.MethodPut, "/api/v1/settings",
		strings.NewReader(`{"key":"otro","value":"x"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("desconocido: quiere 400, got %d", rec.Code)
	}
}

func TestUsageAnalyticsObs(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	if err := db.CreateUser("op", "password-larga-1", store.RoleOperator); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(d)
	tok := login(t, h, "op", "password-larga-1")

	// usage
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/usage?days=30", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body.String())
	}
	var u struct {
		Total   int     `json:"total"`
		ErrRate float64 `json:"err_rate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil || u.Total != 2 || u.ErrRate != 0.5 {
		t.Fatalf("usage mal: %+v %v", u, err)
	}
	// caché: segunda idéntica
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, authed(http.MethodGet, "/api/v1/usage?days=30", tok))
	if rec2.Body.String() != rec.Body.String() {
		t.Fatal("caché inconsistente")
	}

	// analytics global + por host
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/analytics?days=30", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"/a"`) {
		t.Fatalf("analytics: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/analytics?host=nadie&days=30", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":0`) {
		t.Fatalf("analytics host vacío: %d %s", rec.Code, rec.Body.String())
	}

	// observability
	if err := db.InsertEvent("died", "w", "exit 1"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/observability?hours=6", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("obs: %d %s", rec.Code, rec.Body.String())
	}
	var o struct {
		Restarts int      `json:"restarts"`
		Down     []string `json:"down"`
		Buckets  []struct {
			Req int `json:"req"`
			Err int `json:"err"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if o.Restarts != 1 || len(o.Down) != 0 || len(o.Buckets) != 6 {
		t.Fatalf("obs mal: %+v", o)
	}
	reqs := 0
	for _, b := range o.Buckets {
		reqs += b.Req
	}
	if reqs != 2 {
		t.Fatalf("obs buckets suman %d, quieren 2", reqs)
	}
}

func TestDeploymentLog(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	dir := t.TempDir()
	lp := filepath.Join(dir, "w-1.log")
	os.WriteFile(lp, []byte("b1\nb2\n"), 0o644)
	id, err := db.InsertDeployment("w", "abc", lp, "tester", nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, fmt.Sprintf("/api/v1/deployments/%d/log", id), opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "b1") {
		t.Fatalf("log: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/deployments/99999/log", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("fantasma: quiere 404, got %d", rec.Code)
	}
}
