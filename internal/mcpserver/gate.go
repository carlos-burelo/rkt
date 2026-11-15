package mcpserver

// Puerta MCP por herramienta: espejo del RBAC de la API (viewer lee,
// operator muta, admin gestiona usuarios/imports y el sistema).
// Sin entrada en la tabla la herramienta exige admin (fail-closed).
// Las mutaciones se auditan; las lecturas quedan solo en access.log.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt/internal/authz"
	"rkt/internal/logx"
	"rkt/internal/store"
)

// minRoleFor rol mínimo de una tool; desconocida = admin (fail-closed).
// Tabla única en authz.ToolOp.
func minRoleFor(tool string) string {
	op, ok := authz.ToolOp(tool)
	if !ok {
		return string(authz.RoleAdmin)
	}
	return string(authz.MinRole(op))
}

func roleAtLeast(have, want string) bool {
	return authz.AtLeast(authz.Role(have), authz.Role(want))
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// tokenRole resuelve (rol, actor) del Bearer o de la cookie de sesión.
// Con cookie en mutación con Origin de otro host se deniega (CSRF).
func (s *Server) tokenRole(r *http.Request) (string, string) {
	tok := bearerToken(r)
	viaCookie := false
	if tok == "" {
		if c, err := r.Cookie("rkt_token"); err == nil {
			tok, viaCookie = c.Value, true
		}
	}
	if tok == "" {
		return "", ""
	}
	if viaCookie && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if origin := authz.OriginOf(r); origin != "" && !authz.SameOrigin(origin, r.Host, false) {
			return "", ""
		}
	}
	db, err := store.Open(logx.DBPath(s.cfgPath))
	if err != nil {
		return "", ""
	}
	defer db.Close()
	if name, role, ok := db.SessionUser(tok); ok {
		return role, name
	}
	name, role, ok := db.TokenUser(tok)
	if !ok {
		return "", ""
	}
	return role, name
}

type rpcCall struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name string `json:"name"`
	} `json:"params"`
}

// toolCallName extrae (herramienta, id) de un tools/call y restaura el
// body para el handler real. ok=false si no es un tools/call simple
// (listas, notificaciones, batches: pasan sin filtrar).
func toolCallName(r *http.Request) (string, any, bool) {
	if r.Method != http.MethodPost || r.Body == nil {
		return "", nil, false
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return "", nil, false
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(b))
	r.ContentLength = int64(len(b))
	var c rpcCall
	if err := json.Unmarshal(b, &c); err != nil || c.Method != "tools/call" || c.Params.Name == "" {
		return "", nil, false
	}
	return c.Params.Name, c.ID, true
}

func writeRPCError(w http.ResponseWriter, id any, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": -32600, "message": msg},
	})
}

func clientIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// gateHTTP exige el rol de cada herramienta y audita las mutaciones.
// Las lecturas quedan en access.log; sin entrada en la tabla se exige admin.
func (s *Server) gateHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tool, id, ok := toolCallName(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		want := minRoleFor(tool)
		role, actor := s.tokenRole(r)
		if !roleAtLeast(role, want) {
			writeRPCError(w, id, "herramienta "+tool+" requiere rol "+want)
			return
		}
		if want != string(authz.RoleViewer) && actor != "" {
			if db, err := store.Open(logx.DBPath(s.cfgPath)); err == nil {
				_ = db.AddAudit(actor, "mcp "+tool, "", clientIP(r))
				db.Close()
			}
		}
		next.ServeHTTP(w, r)
	})
}

// stdioRole rol del canal stdio: local si lee control.token, viewer si no.
// Sin gate no hay stdio: toda tool pasa por Check con este rol.
func (s *Server) stdioRole() authz.Role {
	b, err := os.ReadFile(logx.ControlTokenPath(s.cfgPath))
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return authz.RoleViewer
	}
	return authz.RoleLocal
}

// stdioCheck autoriza una tool en stdio; nil = pasa (audita mutaciones).
// Op desconocida exige admin (fail-closed): por stdio siempre denegada.
func (s *Server) stdioCheck(tool string) *mcp.CallToolResult {
	op, _ := authz.ToolOp(tool)
	if !authz.Check(s.stdioRole(), op) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "herramienta " + tool + " requiere rol " + string(authz.MinRole(op))}},
			IsError: true,
		}
	}
	if authz.MinRole(op) != authz.RoleViewer {
		if db, err := store.Open(logx.DBPath(s.cfgPath)); err == nil {
			_ = db.AddAudit("stdio", "mcp "+tool, "", "-")
			db.Close()
		}
	}
	return nil
}

// addTool registra una tool; en server stdio la envuelve con el gate.
func addTool[In, Out any](s *Server, srv *mcp.Server, t *mcp.Tool, h mcp.ToolHandlerFor[In, Out]) {
	if !s.stdioGate {
		mcp.AddTool(srv, t, h)
		return
	}
	mcp.AddTool(srv, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		if deny := s.stdioCheck(t.Name); deny != nil {
			var zero Out
			return deny, zero, nil
		}
		return h(ctx, req, in)
	})
}
