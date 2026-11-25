package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"rkt/internal/canary"
	"rkt/internal/geoip"
	"rkt/internal/store"
)

func canaryExe(extra string) []byte {
	var head []byte
	switch runtime.GOOS {
	case "windows":
		head = []byte("MZ\x90\x00")
	case "linux":
		head = []byte{0x7f, 'E', 'L', 'F'}
	default:
		head = []byte{0xcf, 0xfa, 0xed, 0xfe}
	}
	return append(head, []byte(extra)...)
}

// Subir canary: solo admin; el operador lista pero no sube; sha malo = 400.
func TestCanarySoloAdminSube(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.Canary = canary.Open(t.TempDir())
	h := NewHandler(d)
	opTok := loginAs(t, h, db, "op", store.RoleOperator)
	admTok := loginAs(t, h, db, "adm", store.RoleAdmin)

	bin := canaryExe("build-dev")
	sum := sha256.Sum256(bin)
	put := func(tok, sha string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/canary?label=dev-1", bytes.NewReader(bin))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-Rkt-Sha256", sha)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := put(opTok, hex.EncodeToString(sum[:])); rec.Code != http.StatusForbidden {
		t.Fatalf("operator sube: %d, quiere 403", rec.Code)
	}
	if rec := put(admTok, strings.Repeat("0", 64)); rec.Code != http.StatusBadRequest {
		t.Fatalf("sha malo: %d %s", rec.Code, rec.Body.String())
	}
	rec := put(admTok, hex.EncodeToString(sum[:]))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"label":"dev-1"`) {
		t.Fatalf("admin sube: %d %s", rec.Code, rec.Body.String())
	}

	list := httptest.NewRecorder()
	h.ServeHTTP(list, authed(http.MethodGet, "/api/v1/canary", opTok))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), hex.EncodeToString(sum[:])) {
		t.Fatalf("lista: %d %s", list.Code, list.Body.String())
	}

	// aplicar/borrar: operator fuera; id inválido nunca toca disco
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/canary/20260101-000000-aaaaaaaa/apply"},
		{http.MethodDelete, "/api/v1/canary/20260101-000000-aaaaaaaa"},
		{http.MethodPost, "/api/v1/canary/rollback"},
	} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, authed(c.method, c.path, opTok))
		if r.Code != http.StatusForbidden {
			t.Errorf("operator %s %s: %d, quiere 403", c.method, c.path, r.Code)
		}
	}
	r := httptest.NewRecorder()
	h.ServeHTTP(r, authed(http.MethodPost, "/api/v1/canary/..%2f..%2fx/apply", admTok))
	if r.Code != http.StatusNotFound {
		t.Errorf("id con traversal: %d, quiere 404", r.Code)
	}
}

// Guía DNS, canales de correo y mapa de ataques responden con su contrato.
func TestGuiaDNSyAtaques(t *testing.T) {
	d, db, done := testDeps(t)
	defer done()
	d.GeoIP = geoip.Open(t.TempDir() + "/geoip.csv")
	h := NewHandler(d)
	vis := loginAs(t, h, db, "vis", store.RoleViewer)
	op := loginAs(t, h, db, "op", store.RoleOperator)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/guide?host=app.localhost", vis))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"local"`) {
		t.Fatalf("guía: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/domains/guide?host=", vis))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("guía sin host: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/security/attacks", vis))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer ve ataques: %d, quiere 403 (expone IPs)", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed(http.MethodGet, "/api/v1/security/attacks?hours=24", op))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"geoip_ready":false`) {
		t.Fatalf("ataques: %d %s", rec.Code, rec.Body.String())
	}

	// canal de correo sin SMTP: rechazo accionable
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alert-channels",
		strings.NewReader(`{"name":"mail","url":"mailto:ops@example.com"}`))
	req.Header.Set("Authorization", "Bearer "+op)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "rkt set smtp_host") {
		t.Fatalf("canal mailto sin SMTP: %d %s", rec.Code, rec.Body.String())
	}
}
