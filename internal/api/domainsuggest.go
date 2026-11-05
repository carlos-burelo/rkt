// Sugerencia de dominios para entorno local y base propia.
package api

import (
	"net/http"
	"regexp"
	"strings"

	"rkt-proxy/internal/domains"
	"rkt-proxy/internal/store"
)

type suggestOut struct {
	Name           string               `json:"name"`
	Suggestions    []domains.Suggestion `json:"suggestions"`
	Public         bool                 `json:"public"`
	RequiresDomain bool                 `json:"requires_domain"`
	BaseDomain     string               `json:"base_domain,omitempty"`
}

var notSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// handleDomainsSuggest GET /api/v1/domains/suggest?name=app (operator+).
// En local prioriza *.localhost; en servidor público solo ofrece base_domain
// y avisa cuando hace falta pedir un dominio con acceso.
func (d *Deps) handleDomainsSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r) {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	slug := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "falta name")
		return
	}

	used := map[string]bool{}
	for _, rt := range d.Live.Get().Routes {
		for _, h := range rt.Hosts {
			used[strings.ToLower(h)] = true
		}
	}

	base := d.DB.GetSetting(store.KeyBaseDomain)
	publicIP := d.DB.GetSetting(store.KeyPublicIP)
	writeJSON(w, suggestOut{
		Name:           slug,
		Suggestions:    domains.Suggestions(slug, base, publicIP, used),
		Public:         domains.Public(publicIP),
		RequiresDomain: domains.RequiresDomain(publicIP, base),
		BaseDomain:     domains.CleanBase(base),
	})
}
