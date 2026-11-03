package api

import "testing"

func TestDomainSuggestionsPriorizaLocalhost(t *testing.T) {
	got := domainSuggestions("coatl-in", "", map[string]bool{})
	if len(got) != 2 {
		t.Fatalf("sugerencias = %d, quiero 2 (localhost y test)", len(got))
	}
	if got[0].Host != "coatl-in.localhost" || got[0].Kind != "local" {
		t.Fatalf("primera = %+v, quiero .localhost", got[0])
	}
	if got[1].Host != "coatl-in.test" {
		t.Fatalf("segunda = %+v, quiero .test", got[1])
	}
}

func TestDomainSuggestionsUsaBaseYEvitaOcupados(t *testing.T) {
	used := map[string]bool{"coatl-in.localhost": true}
	got := domainSuggestions("coatl-in", "rkt.dev", used)
	if len(got) != 2 {
		t.Fatalf("sugerencias = %+v, quiero test y base", got)
	}
	if got[0].Host != "coatl-in.test" {
		t.Fatalf("primera = %+v, localhost está ocupado", got[0])
	}
	if got[1].Host != "coatl-in.rkt.dev" || got[1].Kind != "base" {
		t.Fatalf("segunda = %+v, quiero dominio base", got[1])
	}
}
