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
	"rkt-proxy/internal/github"
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
		Retry: func(id int64, actor string) (string, error) {
			if id != 7 || actor == "" {
				return "", fmt.Errorf("no repetible")
			}
			return "rev7", nil
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

func TestSetupOneShot(t *testing.T) {
	d, _, done := testDeps(t)
	defer done()
	h := NewHandler(d)

	// status público dice setup requerido
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"setup_required":true`) {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	// setup crea admin + auto-login (cookie)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup",
		strings.NewReader(`{"user":"jefe","password":"password-larga-1"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" || out.Role != "admin" {
		t.Fatalf("setup sin sesión admin: %s", rec.Body.String())
	}
	foundCookie := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "rkt_token" && c.Value != "" && c.HttpOnly {
			foundCookie = true
		}
	}
	if !foundCookie {
		t.Fatal("setup sin cookie httpOnly")
	}
	// segundo setup prohibido (one-shot)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/setup",
		strings.NewReader(`{"user":"otro","password":"password-larga-2"}`))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("segundo setup: quiere 403, got %d", rec.Code)
	}
	// status ya no requiere setup
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil))
	if !strings.Contains(rec.Body.String(), `"setup_required":false`) {
		t.Fatalf("status post-setup: %s", rec.Body.String())
	}
	// sesión del setup opera
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/status", out.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("status con sesión setup: %d", rec.Code)
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
	if !strings.Contains(rec.Body.String(), `"port":""`) {
		t.Fatalf("falta port (vacío honesto): %s", rec.Body.String())
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
	// PUT target (edición inline)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/routes",
		strings.NewReader(`{"host":"a.test","target":"http://127.0.0.1:99"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/routes", opTok))
	if !strings.Contains(rec.Body.String(), "127.0.0.1:99") {
		t.Fatalf("put no aplicado: %s", rec.Body.String())
	}
	// PUT fantasma
	req = httptest.NewRequest(http.MethodPut, "/api/v1/routes",
		strings.NewReader(`{"host":"nadie.test","target":"http://127.0.0.1:99"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("put fantasma: quiere 404, got %d", rec.Code)
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

func TestDeployRetry(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/deployments/7/retry", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "rev7") {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
	}
	// viewer no puede
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/deployments/7/retry", visTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer retry: quiere 403, got %d", rec.Code)
	}
	// id malo
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodPost, "/api/v1/deployments/8/retry", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("retry malo: quiere 400, got %d", rec.Code)
	}
}

func TestAppPutDelete(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// viewer no muta
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/w",
		strings.NewReader(`{"command":"x"}`))
	req.Header.Set("Authorization", "Bearer "+visTok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer put: quiere 403, got %d", rec.Code)
	}
	// inválido rechaza (command vacío)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/apps/w",
		strings.NewReader(`{"command":""}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("put inválido: quiere 400, got %d", rec.Code)
	}
	// válido aplica + visible
	req = httptest.NewRequest(http.MethodPut, "/api/v1/apps/w",
		strings.NewReader(`{"command":"ncmd","restart_delay":"9s"}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w", opTok))
	if !strings.Contains(rec.Body.String(), `"command":"ncmd"`) {
		t.Fatalf("put no aplicado: %s", rec.Body.String())
	}
	// delete + GET 404 después
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/apps/w", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "routes list") {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w", opTok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("post-delete: quiere 404, got %d", rec.Code)
	}
}

func TestAppsCreate(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	post := func(tok, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// viewer no crea
	if rec := post(visTok, `{"name":"n","command":"c"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer alta: quiere 403, got %d", rec.Code)
	}
	// sin datos rechaza
	if rec := post(opTok, `{"name":"","command":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("vacía: quiere 400, got %d", rec.Code)
	}
	// explícita ok
	dir := t.TempDir()
	body := `{"name":"n2","command":"c2","dir":` + strconv_quote(dir) + `}`
	if rec := post(opTok, body); rec.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body.String())
	}
	// duplicada rechaza
	if rec := post(opTok, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("dup: quiere 400, got %d", rec.Code)
	}
	// visible en lista
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps", opTok))
	if !strings.Contains(rec.Body.String(), `"n2"`) {
		t.Fatalf("lista sin n2: %s", rec.Body.String())
	}
}

func TestDomainsTLS(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.TLS = func(host string) (bool, string) {
		if host == "a.test" {
			return true, "2027-01-01"
		}
		return false, ""
	}
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/a.test/tls", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "2027-01-01") {
		t.Fatalf("tls: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/otro.test/tls", opTok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"managed":false`) {
		t.Fatalf("tls off: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAlertsCRUD(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	visTok := loginAs(t, h, db, "vis", store.RoleViewer)

	// viewer lista (vacío ok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/alerts", visTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	// viewer no crea
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts",
		strings.NewReader(`{"app":"*","kind":"failed","threshold":1,"window_min":60}`))
	req.Header.Set("Authorization", "Bearer "+visTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer alta: quiere 403, got %d", rec.Code)
	}
	// kind malo rechaza
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts",
		strings.NewReader(`{"app":"*","kind":"otro","threshold":1,"window_min":60}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("kind malo: quiere 400, got %d", rec.Code)
	}
	// alta ok + lista 1 + baja
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts",
		strings.NewReader(`{"app":"w","kind":"failed","threshold":2,"window_min":30}`))
	req.Header.Set("Authorization", "Bearer "+opTok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID <= 0 {
		t.Fatalf("alta sin id: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/alerts", opTok))
	if !strings.Contains(rec.Body.String(), `"app":"w"`) {
		t.Fatalf("lista sin regla: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodDelete, "/api/v1/alerts/99999", opTok))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("baja fantasma: quiere 400, got %d", rec.Code)
	}
}

func TestProxyStatus(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	// backend caído (puerto cerrado) + TLS apagado (nil)
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/proxy", opTok))
	if rec.Code != http.StatusOK {
		t.Fatalf("proxy: %d", rec.Code)
	}
	var rows []struct {
		Hosts  []string `json:"hosts"`
		Target string   `json:"target"`
		OK     bool     `json:"ok"`
		TLS    []string `json:"tls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Hosts[0] != "a.test" {
		t.Fatalf("filas mal: %+v", rows)
	}
	if rows[0].OK {
		t.Fatal("puerto 9 cerrado debe dar ok=false")
	}
	if len(rows[0].TLS) != 1 || rows[0].TLS[0] != "off" {
		t.Fatalf("tls mal: %+v", rows[0].TLS)
	}
}

func TestGithubEndpoint(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/github", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":false`) {
		t.Fatalf("github sin deps = %d %s", rec.Code, rec.Body.String())
	}

	d.Github = func() (map[string]any, error) {
		return map[string]any{"configured": true, "repository_selection": "all", "repositories": "12"}, nil
	}
	d.GithubRepos = func() ([]github.Repo, error) {
		return []github.Repo{{FullName: "acme/priv", DefaultBranch: "main", Private: true, Owner: "acme"}}, nil
	}
	h = NewHandler(d)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/github", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"repository_selection":"all"`) {
		t.Fatalf("github status = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/github/repos", tok))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"full_name":"acme/priv"`) ||
		!strings.Contains(rec.Body.String(), `"private":true`) {
		t.Fatalf("github repos = %d %s", rec.Code, rec.Body.String())
	}
}

func TestOriginAjenoRechazado(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	req := authed(http.MethodPost, "/api/v1/apps/w/restart", tok)
	req.Header.Set("Origin", "http://evil.test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin ajeno = %d, quiere 403 (%s)", rec.Code, rec.Body.String())
	}

	req = authed(http.MethodPost, "/api/v1/apps/w/restart", tok)
	req.Header.Set("Origin", "http://example.com") // mismo Host que httptest
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("origin propio no debe dar 403: %s", rec.Body.String())
	}
}

func TestUsageEndpoint(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/w/usage?window=60", tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("usage = %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		App     string `json:"app"`
		Running bool   `json:"running"`
		Traffic struct {
			WindowS int `json:"window_s"`
		} `json:"traffic"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if out.App != "w" || !out.Running {
		t.Fatalf("reporte = %+v", out)
	}
	if out.Traffic.WindowS != 60 {
		t.Fatalf("ventana = %d", out.Traffic.WindowS)
	}
}

func TestUsageAppInexistente(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	h := NewHandler(d)
	tok := loginAs(t, h, db, "op", store.RoleOperator)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/apps/nope/usage", tok))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("uso de app inexistente = %d", rec.Code)
	}
}
