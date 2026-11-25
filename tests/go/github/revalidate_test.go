package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGitHub: App instalada en "yo" (all) y "acme" (selected); el usuario
// pertenece también a "nueva-org" donde la App no está.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !bearer {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/app/installations":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 1, "target_type": "User", "repository_selection": "all", "html_url": "u1", "account": map[string]string{"login": "yo"}},
				{"id": 2, "target_type": "Organization", "repository_selection": "selected", "html_url": "u2", "account": map[string]string{"login": "acme"}},
			})
		case "/app/installations/1/access_tokens", "/app/installations/2/access_tokens":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"token": "tok", "expires_at": "2099-01-01T00:00:00Z"})
		case "/installation/repositories":
			json.NewEncoder(w).Encode(map[string]any{"repositories": []map[string]any{{"full_name": "x/y"}, {"full_name": "x/z"}}})
		case "/user":
			if r.Header.Get("Authorization") != "Bearer pat-ok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"id": 10, "login": "yo"})
		case "/user/orgs":
			json.NewEncoder(w).Encode([]map[string]any{{"id": 20, "login": "acme"}, {"id": 30, "login": "Nueva-Org"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRevalidateDetectaOrgSinApp(t *testing.T) {
	_, path := testKey(t)
	srv := fakeGitHub(t)
	defer srv.Close()
	m, err := New(1, path, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := m.Revalidate("mi-app", "pat-ok")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OrgDetection || len(rep.Installations) != 2 || rep.Installations[0].Repos != 2 {
		t.Fatalf("instalaciones: %+v", rep)
	}
	if len(rep.Missing) != 1 || rep.Missing[0].Account != "Nueva-Org" ||
		rep.Missing[0].InstallURL != "https://github.com/apps/mi-app/installations/new/permissions?target_id=30" {
		t.Fatalf("faltantes: %+v", rep.Missing)
	}
	if len(rep.NeedsAll) != 1 || rep.NeedsAll[0] != "acme" {
		t.Fatalf("selected: %+v", rep.NeedsAll)
	}
	if rep.Healthy {
		t.Fatal("con org faltante y selected no está sano")
	}
}

func TestRevalidateSinPAT(t *testing.T) {
	_, path := testKey(t)
	srv := fakeGitHub(t)
	defer srv.Close()
	m, _ := New(1, path, srv.URL)
	rep, err := m.Revalidate("mi-app", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.OrgDetection || len(rep.Missing) != 0 || len(rep.Notes) == 0 || !strings.Contains(rep.Notes[0], "read:org") {
		t.Fatalf("sin PAT debe avisar cómo detectar orgs: %+v", rep)
	}
	// PAT inválido: no rompe, lo dice
	rep, err = m.Revalidate("mi-app", "pat-malo")
	if err != nil || rep.OrgDetection || len(rep.Notes) == 0 {
		t.Fatalf("pat malo: %+v %v", rep, err)
	}
}
