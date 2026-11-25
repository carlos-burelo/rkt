// Tools de acceso a repos privados: credenciales guardadas (PAT / deploy
// key SSH), prueba de acceso y revalidate de la cobertura de la GitHub App.
// Preferida: la GitHub App; PAT y SSH cubren lo que la App no alcanza.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt/internal/deploy"
	"rkt/internal/gitcred"
	"rkt/internal/github"
	"rkt/internal/logx"
)

type gitCredAddArg struct {
	Name   string `json:"name" jsonschema:"nombre (minúsculas, dígitos y ._-)"`
	Kind   string `json:"kind" jsonschema:"pat | ssh"`
	Match  string `json:"match,omitempty" jsonschema:"a qué repos aplica: * | owner/* | owner/repo (default *)"`
	Secret string `json:"secret,omitempty" jsonschema:"pat: el token. ssh: vacío = el servidor genera la llave (recomendado: la privada nunca viaja)"`
}
type gitCredNameArg struct {
	Name string `json:"name" jsonschema:"nombre de la credencial"`
}
type gitAccessArg struct {
	Repo string `json:"repo" jsonschema:"https://github.com/owner/repo"`
}

// ghManager arma el Manager de la GitHub App desde la config (nil si no hay).
func (s *Server) ghManager() (*github.Manager, string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return nil, "", err
	}
	defer db.Close()
	if cfg.Github == nil || cfg.Github.AppID == 0 {
		return nil, "", nil
	}
	gh, err := github.New(cfg.Github.AppID, cfg.Github.KeyFile, "")
	if err != nil {
		return nil, "", err
	}
	slug := db.GetSetting("github_app_slug")
	if slug == "" {
		slug = cfg.Github.AppSlug
	}
	return gh, slug, nil
}

func (s *Server) doGitCredsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	list, err := db.ListGitCreds()
	if err != nil {
		return "", err
	}
	return jsonText(list)
}

func (s *Server) doGitCredAdd(a gitCredAddArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	res, err := gitcred.Add(db, logx.SecretsDir(s.cfgPath), gitcred.AddReq{Name: a.Name, Kind: a.Kind, Match: a.Match, Secret: a.Secret})
	if err != nil {
		return "", err
	}
	return jsonText(res)
}

func (s *Server) doGitCredRemove(name string) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := gitcred.Remove(db, logx.SecretsDir(s.cfgPath), name); err != nil {
		return "", err
	}
	return jsonText(map[string]any{"ok": true})
}

// doGitAccessCheck resuelve con el mismo orden que el deploy (App primero)
// y prueba el acceso real con git ls-remote.
func (s *Server) doGitAccessCheck(ctx context.Context, repo string) (string, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", fmt.Errorf("falta repo")
	}
	gh, _, err := s.ghManager()
	if err != nil {
		return "", err
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	acc, err := gitcred.Resolve(db, logx.SecretsDir(s.cfgPath), gh, repo, "")
	if err != nil {
		return jsonText(map[string]any{"ok": false, "error": err.Error()})
	}
	if acc.SSHKey != "" {
		ctx = deploy.WithSSHKey(ctx, acc.SSHKey)
	}
	out := map[string]any{"via": acc.Via, "note": acc.Note, "ok": true}
	if cerr := deploy.CheckAccess(ctx, repo, acc.Token); cerr != nil {
		out["ok"] = false
		out["error"] = cerr.Error()
		out["hint"] = "instala la GitHub App en el dueño del repo y corre github_revalidate, o guarda un PAT/deploy key con git_cred_add"
	}
	return jsonText(out)
}

func (s *Server) doGithubRevalidate() (string, error) {
	gh, slug, err := s.ghManager()
	if err != nil {
		return "", err
	}
	if gh == nil {
		return "", fmt.Errorf("GitHub App no configurada: créala desde Ajustes → GitHub")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rep, err := gitcred.Revalidate(db, logx.SecretsDir(s.cfgPath), gh, slug)
	if err != nil {
		return "", err
	}
	return jsonText(rep)
}

func (s *Server) registerGit(srv *mcp.Server) {
	add := func(name, desc string, h func(context.Context) (string, error)) {
		addTool(s, srv, &mcp.Tool{Name: name, Description: desc},
			func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
				return text(h(ctx))
			})
	}
	add("git_creds_list", "Credenciales git guardadas (nombre, tipo pat|ssh, patrón, huella). Nunca devuelve secretos.",
		func(context.Context) (string, error) { return s.doGitCredsList() })
	addTool(s, srv, &mcp.Tool{Name: "git_cred_add", Description: "Guarda una credencial para repos privados fuera de la GitHub App. kind=ssh sin secret genera la deploy key en el servidor y devuelve la pública para registrarla en GitHub. kind=pat guarda un token (read:org permite a github_revalidate detectar orgs nuevas). La GitHub App sigue siendo la opción preferida."},
		func(ctx context.Context, req *mcp.CallToolRequest, a gitCredAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doGitCredAdd(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "git_cred_remove", Description: "Borra una credencial git guardada (metadatos y secreto)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a gitCredNameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doGitCredRemove(a.Name))
		})
	addTool(s, srv, &mcp.Tool{Name: "git_access_check", Description: "Dice qué mecanismo usaría el deploy para clonar un repo (App → PAT/SSH guardado → env → anónimo) y prueba el acceso real con git ls-remote. Si falla, da el siguiente paso."},
		func(ctx context.Context, req *mcp.CallToolRequest, a gitAccessArg) (*mcp.CallToolResult, any, error) {
			return text(s.doGitAccessCheck(ctx, a.Repo))
		})
	add("github_revalidate", "Relee las instalaciones de la GitHub App y detecta cuentas/orgs del usuario donde NO está instalada (p. ej. una org nueva) y las instalaciones con repos 'selected'. Devuelve los enlaces de instalación. Para detectar orgs necesita un PAT con read:org guardado (git_cred_add).",
		func(context.Context) (string, error) { return s.doGithubRevalidate() })
}
