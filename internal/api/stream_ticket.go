package api

// Tickets de stream de un solo uso: EventSource no puede mandar
// Authorization, así que un poseedor de rkt_* (sin cookie) pide un ticket
// POST /api/v1/stream/ticket y abre el SSE con ?ticket=. Un uso, 60s.

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
)

type streamTicket struct {
	user    string
	role    string
	expires time.Time
}

var streamTickets = struct {
	sync.Mutex
	m map[string]streamTicket
}{m: map[string]streamTicket{}}

// mintStreamTicket emite un ticket ligado a (usuario, rol).
func mintStreamTicket(user, role string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	t := hex.EncodeToString(b)
	streamTickets.Lock()
	streamTickets.m[t] = streamTicket{user: user, role: role, expires: time.Now().Add(60 * time.Second)}
	if len(streamTickets.m) > 128 {
		for k, v := range streamTickets.m {
			if time.Now().After(v.expires) {
				delete(streamTickets.m, k)
			}
		}
	}
	streamTickets.Unlock()
	return t
}

// takeStreamTicket consume un ticket (un solo uso). ok=false si no existe,
// expiró o ya se usó.
func takeStreamTicket(t string) (user, role string, ok bool) {
	if t == "" {
		return "", "", false
	}
	streamTickets.Lock()
	defer streamTickets.Unlock()
	v, found := streamTickets.m[t]
	if !found || time.Now().After(v.expires) {
		delete(streamTickets.m, t)
		return "", "", false
	}
	delete(streamTickets.m, t)
	return v.user, v.role, true
}

// handleStreamTicket POST /api/v1/stream/ticket (autenticado, cualquier rol).
func (d *Deps) handleStreamTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "solo POST")
		return
	}
	u, role := UserOf(r)
	t := mintStreamTicket(u, role)
	if t == "" {
		writeErr(w, http.StatusInternalServerError, "no pude emitir ticket")
		return
	}
	writeJSON(w, map[string]any{"ticket": t, "expires_in": 60})
}

// streamTicketAuth autentica streams SSE con ?ticket= (un uso).
// Devuelve request con usuario/rol en contexto, como RequireAuth.
func streamTicketAuth(r *http.Request) (*http.Request, bool) {
	p := r.URL.Path
	isStream := p == "/api/v1/fleet/stream" || p == "/api/v1/logs/stream" ||
		(strings.HasPrefix(p, "/api/v1/deployments/") && r.URL.Query().Get("stream") == "1")
	if !isStream {
		return nil, false
	}
	u, role, ok := takeStreamTicket(r.URL.Query().Get("ticket"))
	if !ok {
		return nil, false
	}
	ctx := r.Context()
	ctx = contextWithUser(ctx, u, role)
	return r.WithContext(ctx), true
}
