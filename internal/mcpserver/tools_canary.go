// Tools canary: subir binarios de rkt sin git y aplicarlos (self-update)
// como pasos separados. Subir NO actualiza; aplicar lanza `rkt upgrade
// --local` (gate de salud + rollback automático).
package mcpserver

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt/internal/build"
	"rkt/internal/canary"
	"rkt/internal/logx"
)

type canaryFetchArg struct {
	URL    string `json:"url" jsonschema:"https://... del rkt.exe (solo IPs públicas)"`
	SHA256 string `json:"sha256" jsonschema:"sha256 hex del binario (obligatorio)"`
	Label  string `json:"label,omitempty" jsonschema:"etiqueta, ej: salida de git describe"`
}
type canaryIDArg struct {
	ID string `json:"id" jsonschema:"id del canary (canary_list)"`
}

func (s *Server) canaryStore() *canary.Store { return canary.Open(logx.HomeDir(s.cfgPath)) }

func (s *Server) doCanaryFetch(ctx context.Context, a canaryFetchArg) (string, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	m, err := s.canaryStore().Fetch(c, a.URL, a.Label, a.SHA256, "mcp", time.Now())
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"canary": m, "applied": false, "next": "canary_apply para actualizar"})
}

func (s *Server) doCanaryApply(a canaryIDArg) (string, error) {
	_, path, err := s.canaryStore().Get(a.ID)
	if err != nil {
		return "", err
	}
	home := logx.HomeDir(s.cfgPath)
	if err := canary.Launch(s.cfgPath, home, "--local", path, "--force"); err != nil {
		return "", err
	}
	return jsonText(map[string]any{"ok": true,
		"message": "upgrade lanzado; el servicio se reinicia y la conexión MCP caerá ~1 min. Reconecta y llama canary_status"})
}

func (s *Server) doCanaryRollback() (string, error) {
	if err := canary.Launch(s.cfgPath, logx.HomeDir(s.cfgPath), "--rollback"); err != nil {
		return "", err
	}
	return jsonText(map[string]any{"ok": true, "message": "rollback lanzado; reconecta y llama canary_status"})
}

// doCanaryStatus versión viva, slots y cola del log del último upgrade.
func (s *Server) doCanaryStatus() (string, error) {
	home := logx.HomeDir(s.cfgPath)
	rd := func(p string) string {
		b, _ := os.ReadFile(p)
		return strings.TrimSpace(string(b))
	}
	logTail := rd(canary.LogPath(home))
	if lines := strings.Split(logTail, "\n"); len(lines) > 25 {
		logTail = strings.Join(lines[len(lines)-25:], "\n")
	}
	return jsonText(map[string]any{
		"version":     build.Version,
		"current":     rd(logx.CurrentFile(home)),
		"previous":    rd(logx.PreviousFile(home)),
		"canaries":    s.canaryStore().List(),
		"upgrade_log": logTail,
	})
}

func (s *Server) registerCanary(srv *mcp.Server) {
	addTool(s, srv, &mcp.Tool{Name: "canary_list", Description: "Canaries de rkt subidos (id, etiqueta, sha256, tamaño). Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(jsonText(s.canaryStore().List()))
		})
	addTool(s, srv, &mcp.Tool{Name: "canary_status", Description: "Versión viva, slot actual/previo y log del último upgrade. Úsalo tras canary_apply (la conexión cae al reiniciar)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doCanaryStatus())
		})
	addTool(s, srv, &mcp.Tool{Name: "canary_fetch", Description: "SUBE un canary: el servidor descarga el rkt.exe de una URL https (sha256 obligatorio) y lo guarda. NO actualiza. Para subir desde tu máquina usa: rkt canary push."},
		func(ctx context.Context, req *mcp.CallToolRequest, a canaryFetchArg) (*mcp.CallToolResult, any, error) {
			return text(s.doCanaryFetch(ctx, a))
		})
	addTool(s, srv, &mcp.Tool{Name: "canary_apply", Description: "Self-update: aplica un canary subido (rkt upgrade --local --force con gate de salud y rollback automático). Reinicia el servicio."},
		func(ctx context.Context, req *mcp.CallToolRequest, a canaryIDArg) (*mcp.CallToolResult, any, error) {
			return text(s.doCanaryApply(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "canary_discard", Description: "Borra un canary subido por id."},
		func(ctx context.Context, req *mcp.CallToolRequest, a canaryIDArg) (*mcp.CallToolResult, any, error) {
			if err := s.canaryStore().Remove(a.ID); err != nil {
				return text("", err)
			}
			return text(jsonText(map[string]any{"ok": true}))
		})
	addTool(s, srv, &mcp.Tool{Name: "canary_rollback", Description: "Vuelve al slot previo (rkt upgrade --rollback). Reinicia el servicio."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doCanaryRollback())
		})
}
