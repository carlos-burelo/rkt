package gitcred

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"rkt/internal/github"
	"rkt/internal/store"
)

// AddReq alta de credencial. Kind ssh sin Secret genera la llave en el
// servidor (la privada nunca viaja); con Secret importa una existente.
type AddReq struct {
	Name   string
	Kind   string
	Match  string
	Secret string
}

// AddResult credencial creada. DeployKey es la pública que se registra en
// GitHub (Settings → Deploy keys, o en la cuenta del usuario); solo se
// entrega en el alta.
type AddResult struct {
	Cred      store.GitCred `json:"credential"`
	DeployKey string        `json:"deploy_key,omitempty"`
	Next      string        `json:"next"`
}

// Add valida, guarda el secreto (0600) y los metadatos. Si el registro en
// DB falla, borra el secreto: nunca queda uno huérfano.
func Add(db *store.DB, secretsDir string, r AddReq) (AddResult, error) {
	if !ValidName(r.Name) {
		return AddResult{}, fmt.Errorf("nombre inválido %q: minúsculas, dígitos y ._- (máx 48)", r.Name)
	}
	if r.Match == "" {
		r.Match = "*"
	}
	if err := ValidMatch(r.Match); err != nil {
		return AddResult{}, err
	}
	existing, err := db.ListGitCreds()
	if err != nil {
		return AddResult{}, err
	}
	for _, c := range existing {
		if c.Name == r.Name {
			return AddResult{}, fmt.Errorf("la credencial %q ya existe: bórrala antes (git_cred_remove) o usa otro nombre", r.Name)
		}
	}
	cred := store.GitCred{Name: r.Name, Kind: r.Kind, Match: r.Match}
	var secret []byte
	var res AddResult
	switch r.Kind {
	case KindPAT:
		if err := ValidPAT(r.Secret); err != nil {
			return AddResult{}, err
		}
		secret = []byte(strings.TrimSpace(r.Secret) + "\n")
		res.Next = "prueba el acceso con git_access_check; para detectar orgs nuevas el PAT necesita read:org"
	case KindSSH:
		var pub, fp string
		var err error
		if strings.TrimSpace(r.Secret) == "" {
			secret, pub, fp, err = NewDeployKey("rkt-" + r.Name)
		} else {
			secret = []byte(strings.TrimSpace(r.Secret) + "\n")
			pub, fp, err = InspectPrivateKey(secret)
		}
		if err != nil {
			return AddResult{}, err
		}
		cred.PublicKey, cred.Fingerprint = pub, fp
		res.DeployKey = pub
		res.Next = "registra la llave pública como Deploy key del repo en GitHub (Settings → Deploy keys) y prueba con git_access_check"
	default:
		return AddResult{}, fmt.Errorf("kind inválido %q: pat | ssh", r.Kind)
	}
	if err := WriteSecret(secretsDir, r.Name, r.Kind, secret); err != nil {
		return AddResult{}, err
	}
	if err := db.AddGitCred(cred); err != nil {
		_ = RemoveSecret(secretsDir, r.Name, r.Kind)
		return AddResult{}, err
	}
	res.Cred = cred
	return res, nil
}

// Remove borra metadatos y secreto.
func Remove(db *store.DB, secretsDir, name string) error {
	creds, err := db.ListGitCreds()
	if err != nil {
		return err
	}
	for _, c := range creds {
		if c.Name == name {
			if err := db.DeleteGitCred(name); err != nil {
				return err
			}
			return RemoveSecret(secretsDir, name, c.Kind)
		}
	}
	return fmt.Errorf("credencial %q inexistente", name)
}

// Material devuelve el secreto utilizable de una credencial por nombre.
func Material(db *store.DB, secretsDir, name string) (token, sshKey string, err error) {
	creds, err := db.ListGitCreds()
	if err != nil {
		return "", "", err
	}
	for _, c := range creds {
		if c.Name != name {
			continue
		}
		if c.Kind == KindSSH {
			return "", KeyPath(secretsDir, name), nil
		}
		tok, err := ReadPAT(secretsDir, name)
		return tok, "", err
	}
	return "", "", fmt.Errorf("credencial %q inexistente", name)
}

// UserPAT primer PAT guardado (prefiere el de patrón "*"), "" si no hay.
// Sirve para hablar con la API como el usuario (listar sus orgs).
func UserPAT(db *store.DB, secretsDir string) string {
	creds, err := db.ListGitCreds()
	if err != nil {
		return ""
	}
	var pats []store.GitCred
	for _, c := range creds {
		if c.Kind == KindPAT {
			pats = append(pats, c)
		}
	}
	sort.SliceStable(pats, func(i, j int) bool { return pats[i].Match == "*" && pats[j].Match != "*" })
	for _, c := range pats {
		if tok, err := ReadPAT(secretsDir, c.Name); err == nil && tok != "" {
			return tok
		}
	}
	return ""
}

// Revalidate re-comprueba la cobertura de la GitHub App con el PAT del
// usuario (si hay) para detectar orgs nuevas sin la App instalada.
func Revalidate(db *store.DB, secretsDir string, m *github.Manager, slug string) (github.Report, error) {
	return m.Revalidate(slug, UserPAT(db, secretsDir))
}

// Access credencial elegida para clonar un repo y por qué.
type Access struct {
	Token  string `json:"-"`   // https (App, PAT o env); nunca se serializa
	SSHKey string `json:"-"`   // ruta de la deploy key
	Via    string `json:"via"` // github-app | pat:<n> | ssh:<n> | env | anonymous | ssh-url
	// Note por qué la App no cubrió el repo (cuando se cayó a otro mecanismo).
	Note string `json:"note,omitempty"`
}

// Resolve hogar único del orden de preferencia para repos privados:
//  1. GitHub App instalada en el dueño del repo (la preferida),
//  2. credencial guardada que mejor encaje (repo exacto > owner/* > *),
//  3. token de entorno heredado (RKT_GITHUB_TOKEN_FILE / tokenEnv),
//  4. anónimo (repos públicos).
//
// Que la App no cubra el repo (org nueva sin instalar) no es error: se cae
// al siguiente mecanismo y queda en Note. gh puede ser nil.
func Resolve(db *store.DB, secretsDir string, gh *github.Manager, repoURL, tokenEnv string) (Access, error) {
	if !strings.HasPrefix(repoURL, "http://") && !strings.HasPrefix(repoURL, "https://") {
		return Access{Via: "ssh-url"}, nil // git@host:o/r usa el ssh del sistema
	}
	owner, repo, ok := github.ParseRepo(repoURL)
	if !ok {
		return Access{}, fmt.Errorf("repo no parseable %q", repoURL)
	}
	note := ""
	if gh != nil {
		id, _, err := gh.InstallationForRepo(owner, repo)
		if err == nil {
			tok, terr := gh.TokenFor(id)
			if terr == nil {
				return Access{Token: tok, Via: "github-app"}, nil
			}
			note = "token de la GitHub App falló: " + terr.Error()
		} else {
			note = "la GitHub App no cubre " + owner + "/" + repo + " (¿org nueva? corre github_revalidate): " + err.Error()
		}
	}
	if creds, err := db.ListGitCreds(); err == nil {
		if c := Pick(creds, owner, repo); c != nil {
			tok, key, err := Material(db, secretsDir, c.Name)
			if err != nil {
				return Access{}, err
			}
			via := "pat:" + c.Name
			if key != "" {
				via = "ssh:" + c.Name
			}
			return Access{Token: tok, SSHKey: key, Via: via, Note: note}, nil
		}
	}
	if file := os.Getenv("RKT_GITHUB_TOKEN_FILE"); file != "" {
		if b, err := os.ReadFile(file); err == nil {
			if t := strings.TrimSpace(string(b)); t != "" {
				return Access{Token: t, Via: "env", Note: note}, nil
			}
		}
	}
	if tokenEnv != "" {
		if t := os.Getenv(tokenEnv); t != "" {
			return Access{Token: t, Via: "env", Note: note}, nil
		}
	}
	return Access{Via: "anonymous", Note: note}, nil
}
