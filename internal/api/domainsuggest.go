// Sugerencia de dominios para entorno local y base propia.
package api

import (
	"net/http"
	"regexp"
	"strings"

	"rkt-proxy/internal/store"
)

type domainSuggestion struct {
	Host string `json:"host"`
	Kind string `json:"kind"`
	Note string `json:"note"`
}

type suggestOut struct {
	Name        string             `json:"name"`
	Suggestions []domainSuggestion `json:"suggestions"`
}

var notSlug = regexp.MustCompile(`[^a-z0-9-]+`)

// handleDomainsSuggest GET /api/v1/domains/suggest?name=app (operator+).
// Prioriza lo que funciona sin tocar nada: *.localhost resuelve a loopback
// en navegadores modernos; el resto se declara con su requisito.
func (d *Deps) handleDomainsSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "solo GET")
		return
	}
	if !requireOperator(w, r, d.DB) {
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

	out := suggestOut{Name: slug, Suggestions: domainSuggestions(slug, d.DB.GetSetting(store.KeyBaseDomain), used)}
	writeJSON(w, out)
}

// domainSuggestions prioriza lo que funciona sin configurar nada.
// Puro: testeable sin HTTP ni DB.
func domainSuggestions(slug, base string, used map[string]bool) []domainSuggestion {
	out := []domainSuggestion{}
	add := func(host, kind, note string) {
		if host == "" || used[strings.ToLower(host)] {
			return
		}
		out = append(out, domainSuggestion{Host: host, Kind: kind, Note: note})
	}
	add(slug+".localhost", "local", "funciona sin editar hosts")
	add(slug+".test", "local", "requiere hosts → 127.0.0.1")
	if base = strings.TrimSpace(base); base != "" {
		base = strings.TrimPrefix(strings.TrimSuffix(base, "."), ".")
		add(slug+"."+base, "base", "tu dominio base")
	}
	return out
}
