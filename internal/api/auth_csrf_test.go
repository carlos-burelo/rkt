package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// El check CSRF compara el Origin con el Host. En dev (LocalCA) localhost/
// 127.0.0.1/::1 cuentan como mismo origen con cualquier puerto (proxy Vite
// manda Origin localhost:5173 y el backend ve 127.0.0.1:8090). En producción
// la comparación es exacta por host + puerto: otro puerto loopback no puede
// mutar (SameSite=Lax no distingue puertos).
func TestRequireAuthOrigin(t *testing.T) {
	_, db, done := testDeps(t)
	defer done()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	t.Run("dev relaja loopback", func(t *testing.T) {
		h := RequireAuth(db, func() bool { return true }, next)
		pasar := [][2]string{
			{"http://127.0.0.1:8090", "127.0.0.1:8090"},
			{"http://localhost:5173", "127.0.0.1:8090"},
			{"http://[::1]:5173", "[::1]:8090"},
			{"", "127.0.0.1:8090"}, // sin Origin (curl/MCP): pasa el gate
		}
		for _, tc := range pasar {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
			req.Host = tc[1]
			if tc[0] != "" {
				req.Header.Set("Origin", tc[0])
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			// Sin token: 401 (pasó el gate CSRF, falló la auth).
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("origin %q host %q: got %d, quiere 401 (gate pasado)", tc[0], tc[1], rec.Code)
			}
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
		req.Host = "127.0.0.1:8090"
		req.Header.Set("Origin", "https://evil.test")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("origen externo: got %d, quiere 403", rec.Code)
		}
	})

	t.Run("prod exige origen exacto", func(t *testing.T) {
		h := RequireAuth(db, func() bool { return false }, next)
		// otro puerto loopback: debe rechazarse (CSRF entre puertos)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
		req.Host = "127.0.0.1:8090"
		req.Header.Set("Origin", "http://127.0.0.1:9999")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("otro puerto loopback: got %d, quiere 403", rec.Code)
		}
		// mismo origen: pasa el gate (401 por falta de token)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
		req.Host = "127.0.0.1:8090"
		req.Header.Set("Origin", "http://127.0.0.1:8090")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("mismo origen: got %d, quiere 401", rec.Code)
		}
		// sin Origin (curl/MCP): pasa el gate
		req = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
		req.Host = "127.0.0.1:8090"
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("sin Origin: got %d, quiere 401", rec.Code)
		}
	})
}
