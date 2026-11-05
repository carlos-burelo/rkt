package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"rkt-proxy/internal/store"
)

// ctxUser/ctxRole usuario autenticado en el request.
type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxRole
)

// UserOf devuelve (usuario, rol) del request autenticado.
func UserOf(r *http.Request) (string, string) {
	u, _ := r.Context().Value(ctxUser).(string)
	role, _ := r.Context().Value(ctxRole).(string)
	return u, role
}

func roleLevel(r string) int {
	switch r {
	case store.RoleAdmin:
		return 3
	case store.RoleOperator:
		return 2
	case store.RoleViewer:
		return 1
	}
	return 0
}

func bearerOf(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie("rkt_token"); err == nil {
		return c.Value
	}
	return ""
}

// RequireAuth cualquier sesión válida. Inyecta usuario/rol en contexto.
// Rechaza mutaciones con Origin de otro host (defensa CSRF además de SameSite).
func RequireAuth(db *store.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) {
					writeErr(w, http.StatusForbidden, "origen no permitido")
					return
				}
			}
		}
		tok := bearerOf(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "falta credencial (login)")
			return
		}
		user, role, ok := db.SessionUser(tok)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "sesión inválida o expirada")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUser, user)
		ctx = context.WithValue(ctx, ctxRole, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type loginThrottle struct {
	mu   sync.Mutex
	hits map[string]*attempt
}

type attempt struct {
	count int
	until time.Time
}

func (t *loginThrottle) blocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.hits[ip]
	if !ok {
		return false
	}
	if time.Now().After(a.until) && a.count < 5 {
		return false
	}
	return time.Now().Before(a.until)
}

func (t *loginThrottle) fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hits == nil {
		t.hits = map[string]*attempt{}
	}
	a := t.hits[ip]
	if a == nil {
		a = &attempt{}
		t.hits[ip] = a
	}
	a.count++
	if a.count >= 5 {
		a.until = time.Now().Add(time.Minute)
	}
	// techo anti-memoria: purga simple
	if len(t.hits) > 1000 {
		for k, v := range t.hits {
			if time.Now().After(v.until) && v.count < 5 {
				delete(t.hits, k)
			}
		}
	}
}

func (t *loginThrottle) ok(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.hits, ip)
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// handleAuthStatus informa si falta crear el primer admin (público).
func (d *Deps) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	n, err := d.DB.CountUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"setup_required": n == 0})
}

// handleSetup crea el primer admin y abre sesión (one-shot).
func (d *Deps) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	n, err := d.DB.CountUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n > 0 {
		writeErr(w, http.StatusForbidden, "setup ya completado")
		return
	}
	ip := clientIP(r)
	if d.th.blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "demasiados intentos, espera 60s")
		return
	}
	var in struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	// re-chequea bajo carrera: el primero gana, el resto 403.
	// validación primero para error 400 exacto (no mentir).
	if err := store.ValidateCredential(in.User, in.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.DB.CreateUser(in.User, in.Password, store.RoleAdmin); err != nil {
		d.th.fail(ip)
		writeErr(w, http.StatusForbidden, "setup ya completado")
		return
	}
	d.th.ok(ip)
	tok, err := d.DB.NewSession(in.User, store.SessionTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "rkt_token", Value: tok, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int((store.SessionTTL).Seconds()),
	})
	writeJSON(w, map[string]any{"user": in.User, "role": store.RoleAdmin, "token": tok})
}

// handleLogin valida credenciales y abre sesión bearer + cookie httpOnly.
func (d *Deps) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	ip := clientIP(r)
	if d.th.blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "demasiados intentos, espera 60s")
		return
	}
	var in struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	role, err := d.DB.CheckPassword(in.User, in.Password)
	if err != nil {
		d.th.fail(ip)
		writeErr(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}
	d.th.ok(ip)
	tok, err := d.DB.NewSession(in.User, store.SessionTTL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "rkt_token", Value: tok, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: int((store.SessionTTL).Seconds()),
	})
	writeJSON(w, map[string]any{"user": in.User, "role": role, "token": tok})
}

// handleLogout invalida la sesión y borra la cookie.
func (d *Deps) handleLogout(w http.ResponseWriter, r *http.Request) {
	if tok := bearerOf(r); tok != "" {
		d.DB.DeleteSession(tok)
	}
	http.SetCookie(w, &http.Cookie{Name: "rkt_token", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]string{"ok": "true"})
}

// handleMe devuelve el usuario de la sesión.
func (d *Deps) handleMe(w http.ResponseWriter, r *http.Request) {
	u, role := UserOf(r)
	writeJSON(w, map[string]string{"user": u, "role": role})
}

// handleUsers lista o crea usuarios (solo admin).
func (d *Deps) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		users, err := d.DB.ListUsers()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, users)
	case http.MethodPost:
		var in struct {
			Name     string `json:"name"`
			Password string `json:"password"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		if err := d.DB.CreateUser(in.Name, in.Password, in.Role); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"ok": "true"})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET|POST")
	}
}

// handleUser borra un usuario por nombre (solo admin).
func (d *Deps) handleUser(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "solo DELETE")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if name == "" || strings.Contains(name, "/") {
		writeErr(w, http.StatusBadRequest, "nombre inválido")
		return
	}
	if err := d.DB.DeleteUser(name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}

// mustUser usuario autenticado (ruta ya pasó RequireAuth).
func mustUser(r *http.Request) string {
	u, _ := UserOf(r)
	return u
}

// requireOperator exige rol operator+ (escribe 403 si no).
func requireOperator(w http.ResponseWriter, r *http.Request) bool {
	_, role := UserOf(r)
	if roleLevel(role) < roleLevel(store.RoleOperator) {
		writeErr(w, http.StatusForbidden, "requiere rol operator")
		return false
	}
	return true
}

// requireAdmin exige rol admin (gestión de usuarios).
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	_, role := UserOf(r)
	if roleLevel(role) < roleLevel(store.RoleAdmin) {
		writeErr(w, http.StatusForbidden, "requiere rol admin")
		return false
	}
	return true
}
