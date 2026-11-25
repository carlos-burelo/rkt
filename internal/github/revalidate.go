package github

import (
	"fmt"
	"sort"
	"strings"
)

// InstallStatus una instalación de la App con su cobertura.
type InstallStatus struct {
	ID          int64  `json:"id"`
	Account     string `json:"account"`
	Type        string `json:"type"`      // User | Organization
	Selection   string `json:"selection"` // all | selected
	Repos       int    `json:"repos"`
	SettingsURL string `json:"settings_url,omitempty"`
}

// MissingAccount cuenta del usuario donde la App aún no está instalada.
type MissingAccount struct {
	Account    string `json:"account"`
	Type       string `json:"type"`
	InstallURL string `json:"install_url"`
}

// Report resultado de revalidar: qué cubre la App y qué falta.
type Report struct {
	Installations []InstallStatus  `json:"installations"`
	Missing       []MissingAccount `json:"missing"`
	NeedsAll      []string         `json:"needs_all_repos"` // instalaciones con "selected"
	OrgDetection  bool             `json:"org_detection"`   // hubo PAT con read:org
	Notes         []string         `json:"notes"`
	Healthy       bool             `json:"healthy"`
}

type userResp struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// Revalidate re-lee las instalaciones de la App y, si hay un PAT del
// usuario (read:org), contrasta con sus cuentas y orgs reales para
// encontrar donde falta instalarla (p. ej. una org creada después).
// Sin PAT no puede enumerar orgs: lo dice en Notes y apunta a la
// instalación manual. slug = app_slug de la App (URL de instalación).
func (m *Manager) Revalidate(slug, userToken string) (Report, error) {
	rep := Report{Installations: []InstallStatus{}, Missing: []MissingAccount{}, NeedsAll: []string{}, Notes: []string{}}
	insts, err := m.Installations()
	if err != nil {
		return rep, err
	}
	covered := map[string]bool{}
	for _, in := range insts {
		st := InstallStatus{
			ID: in.ID, Account: in.Account.Login, Type: in.TargetType,
			Selection: in.RepositorySelection, SettingsURL: in.HTMLURL,
		}
		if tok, terr := m.TokenFor(in.ID); terr == nil {
			if repos, rerr := m.Repositories(tok); rerr == nil {
				st.Repos = len(repos)
			}
		} else {
			rep.Notes = append(rep.Notes, fmt.Sprintf("%s: token de instalación falló: %v", st.Account, terr))
		}
		covered[strings.ToLower(st.Account)] = true
		if st.Selection != "all" {
			rep.NeedsAll = append(rep.NeedsAll, st.Account)
		}
		rep.Installations = append(rep.Installations, st)
	}
	sort.Slice(rep.Installations, func(i, j int) bool { return rep.Installations[i].Account < rep.Installations[j].Account })

	installBase := "https://github.com/apps/" + slug + "/installations/new"
	if userToken == "" {
		rep.Notes = append(rep.Notes,
			"No puedo detectar orgs nuevas sin un PAT con read:org: `rkt git cred add --name yo --kind pat --match '*' --secret-file F`. "+
				"Mientras, instala la App en la org desde "+installBase)
	} else {
		var me userResp
		var orgs []userResp
		if err := m.getJSON(m.apiBase+"/user", userToken, &me); err != nil {
			rep.Notes = append(rep.Notes, "el PAT no autentica: "+err.Error())
		} else if err := m.getJSON(m.apiBase+"/user/orgs?per_page=100", userToken, &orgs); err != nil {
			rep.Notes = append(rep.Notes, "el PAT no puede listar orgs (falta read:org): "+err.Error())
		} else {
			rep.OrgDetection = true
			add := func(u userResp, typ string) {
				if !covered[strings.ToLower(u.Login)] {
					rep.Missing = append(rep.Missing, MissingAccount{
						Account: u.Login, Type: typ,
						InstallURL: fmt.Sprintf("%s/permissions?target_id=%d", installBase, u.ID),
					})
				}
			}
			add(me, "User")
			for _, o := range orgs {
				add(o, "Organization")
			}
			if len(orgs) == 100 {
				rep.Notes = append(rep.Notes, "más de 100 orgs: la lista se corta en 100")
			}
		}
	}
	rep.Healthy = len(rep.Missing) == 0 && len(rep.NeedsAll) == 0 && len(rep.Installations) > 0
	return rep, nil
}
