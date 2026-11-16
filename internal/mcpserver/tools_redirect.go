// Tools de redirecciones: trazado de cadenas y chequeo del setup de GitHub.
// El caso que las motivó: el manifest llevaba redirect_url con localhost +
// puerto explícito y GitHub lo rechazaba ("must be a valid URL").
package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt/internal/github"
	"rkt/internal/store"
)

type traceArg struct {
	URL     string `json:"url" jsonschema:"URL inicial a trazar (http/https)"`
	Method  string `json:"method,omitempty" jsonschema:"GET o HEAD (default GET)"`
	MaxHops int    `json:"max_hops,omitempty" jsonschema:"tope de saltos (default 10, máx 20)"`
}

type traceHop struct {
	N        int    `json:"n"`
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Location string `json:"location,omitempty"`
	Note     string `json:"note,omitempty"`
}

// doTraceRedirects sigue una cadena sin auto-follow: cada salto queda
// registrado (útil para /_rkt/setup, callbacks y rutas del proxy).
func (s *Server) doTraceRedirects(a traceArg) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(a.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		return "", fmt.Errorf("method debe ser GET o HEAD")
	}
	maxHops := a.MaxHops
	if maxHops <= 0 {
		maxHops = 10
	}
	if maxHops > 20 {
		maxHops = 20
	}
	cur, err := url.Parse(strings.TrimSpace(a.URL))
	if err != nil || (cur.Scheme != "http" && cur.Scheme != "https") || cur.Host == "" {
		return "", fmt.Errorf("url debe ser http(s) válida")
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	seen := map[string]bool{}
	hops := []traceHop{}
	verdict := ""
	final := ""
	for n := 1; n <= maxHops; n++ {
		req, err := http.NewRequest(method, cur.String(), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "rkt-mcp/1")
		req.Header.Set("Accept", "*/*")
		resp, err := client.Do(req)
		if err != nil {
			hops = append(hops, traceHop{N: n, URL: cur.String(), Note: "error de red: " + err.Error()})
			verdict = "network_error"
			break
		}
		resp.Body.Close()
		h := traceHop{N: n, URL: cur.String(), Status: resp.StatusCode}
		if resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusFound &&
			resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusTemporaryRedirect &&
			resp.StatusCode != http.StatusPermanentRedirect {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				verdict = "ok"
			} else {
				verdict = fmt.Sprintf("final_status_%d", resp.StatusCode)
			}
			final = cur.String()
			hops = append(hops, h)
			break
		}
		loc := resp.Header.Get("Location")
		if loc == "" {
			h.Note = "3xx sin Location"
			verdict = "empty_location"
			final = cur.String()
			hops = append(hops, h)
			break
		}
		next, err := cur.Parse(loc)
		if err != nil {
			// En la práctica client.Do ya rechaza Location inparseable
			// (llega como error de red); esto es defensa en profundidad.
			h.Note = "Location inválida"
			h.Location = loc
			verdict = "invalid_location"
			final = cur.String()
			hops = append(hops, h)
			break
		}
		h.Location = next.String()
		if next.Scheme != "http" && next.Scheme != "https" {
			h.Note = "esquema no http(s), se detiene"
			verdict = "non_http_target"
			final = next.String()
			hops = append(hops, h)
			break
		}
		if cur.Scheme == "https" && next.Scheme == "http" {
			h.Note = "bajada https→http"
		}
		hops = append(hops, h)
		if seen[next.String()] {
			verdict = "redirect_loop"
			final = next.String()
			break
		}
		seen[next.String()] = true
		cur = next
		if n == maxHops {
			verdict = "too_many_redirects"
			final = cur.String()
		}
	}
	if verdict == "" {
		verdict = "too_many_redirects"
		final = cur.String()
	}
	return jsonText(map[string]any{
		"url": a.URL, "method": method, "verdict": verdict,
		"hops": hops, "hops_count": len(hops), "final_url": final,
	})
}

// checkManifestRedirectURL aplica las reglas que GitHub exige al
// redirect_url del manifest (verificadas contra github.com):
// URL http(s) válida; localhost con puerto explícito lo rechaza.
func checkManifestRedirectURL(raw string) []string {
	problems := []string{}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return []string{"no es una URL http(s) válida"}
	}
	if strings.EqualFold(u.Hostname(), "localhost") && u.Port() != "" {
		problems = append(problems, "localhost con puerto explícito: GitHub lo rechaza (usa 127.0.0.1)")
	}
	return problems
}

// doGithubSetupCheck valida el redirect_url del manifest con la config
// efectiva (base_domain o loopback), sin red: no filtra el state one-time.
func (s *Server) doGithubSetupCheck() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	origin, domain, local, tls := github.SetupOrigin(
		db.GetSetting(store.KeyBaseDomain), cfg.ListenHTTP, cfg.ListenHTTPS)
	redirectURL := origin + "/_rkt/setup"
	problems := checkManifestRedirectURL(redirectURL)
	verdict := "ok"
	if len(problems) > 0 {
		verdict = "needs_attention"
	}
	return jsonText(map[string]any{
		"origin": origin, "domain": domain, "local": local, "tls": tls,
		"redirect_url": redirectURL, "problems": problems, "verdict": verdict,
	})
}

// registerRedirect añade tools de redirecciones al monitoreo.
func (s *Server) registerRedirect(srv *mcp.Server) {
	addTool(s, srv, &mcp.Tool{Name: "trace_redirects", Description: "Traza una cadena de redirects salto a salto (loop, exceso, Location inválida, bajada https→http). Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a traceArg) (*mcp.CallToolResult, any, error) {
			return text(s.doTraceRedirects(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "github_setup_check", Description: "Valida el redirect_url del manifest de GitHub App con la config efectiva (reglas verificadas contra github.com). Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doGithubSetupCheck())
		})
}
