// Package pg gestiona el Postgres local compartido: una DB para todas las
// apps, un esquema por app (role propio o compartido, a elegir).
// Todo lo que toca el servidor vive en el servicio (slice 2); aquí solo
// hay construcción pura de SQL/nombres/rutas (testeable sin Postgres).
package pg

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Defaults del Postgres gestionado.
const (
	DefaultPort     = 5432
	DefaultDatabase = "rkt"
	// SharedRole rol común del modo compartido (un usuario para todo).
	SharedRole = "rkt_apps"
)

// Bins herramientas necesarias (initdb, pg_ctl, psql, pg_dump, pg_restore).
type Bins struct {
	Initdb    string
	PgCtl     string
	Psql      string
	PgDump    string
	PgRestore string
}

// requiredTools binarios exigidos para operar (psql/pg_dump/pg_restore).
// initdb/pg_ctl son opcionales: el cluster lo instala el operador,
// rkt solo aprovisiona encima.
var requiredTools = []string{"psql", "pg_dump", "pg_restore"}

var optionalTools = []string{"initdb", "pg_ctl"}

// ValidIdent verifica un identificador Postgres sin comillas:
// [a-z_][a-z0-9_$]*, máx 63. Sin mayúsculas: Postgres las pliega a
// minúsculas cuando no se comilla, y aquí siempre comillamos con el
// nombre tal cual, así que se exigen minúsculas desde el inicio.
func ValidIdent(s string) error {
	if len(s) == 0 || len(s) > 63 {
		return fmt.Errorf("identificador de 1..63 caracteres")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || c == '$' ||
			(c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9')
		if !ok {
			return fmt.Errorf("usa [a-z_][a-z0-9_$]* sin mayúsculas")
		}
	}
	return nil
}

// QuoteIdent comilla un identificador (duplica comillas internas).
func QuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// SchemaForApp deriva el esquema de una app ([a-z0-9-]): minúsculas,
// guiones a subrayados. Determinista y documentado.
func SchemaForApp(app string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(app))
	if s == "" {
		return "", fmt.Errorf("nombre de app vacío")
	}
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
			b.WriteRune(c)
		case c == '-':
			b.WriteRune('_')
		default:
			return "", fmt.Errorf("nombre de app inválido %q: usa [a-z0-9-]", app)
		}
	}
	out := b.String()
	if len(out) > 63 {
		return "", fmt.Errorf("esquema derivado demasiado largo (máx 63)")
	}
	return out, nil
}

// ResolveBin localiza las herramientas. setting (estilo node_bin) fija
// el directorio; si falta, se busca en PATH. Solo psql/pg_dump son
// obligatorios; initdb/pg_ctl quedan vacíos si no están.
func ResolveBin(setting string) (Bins, error) {
	look := func(name string) (string, error) {
		if setting != "" {
			cand := filepath.Join(filepath.Dir(setting), name+exeSuffix())
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand, nil
			}
			return "", fmt.Errorf("%s no está junto a %s", name, setting)
		}
		return exec.LookPath(name)
	}
	var b Bins
	dst := map[string]*string{"initdb": &b.Initdb, "pg_ctl": &b.PgCtl, "psql": &b.Psql, "pg_dump": &b.PgDump, "pg_restore": &b.PgRestore}
	var missing []string
	for _, t := range requiredTools {
		p, err := look(t)
		if err != nil {
			missing = append(missing, t)
			continue
		}
		*dst[t] = p
	}
	if len(missing) > 0 {
		return Bins{}, fmt.Errorf("faltan binarios: %s", strings.Join(missing, ", "))
	}
	for _, t := range optionalTools {
		if p, err := look(t); err == nil {
			*dst[t] = p
		}
	}
	return b, nil
}

func exeSuffix() string {
	if filepath.Separator == '\\' {
		return ".exe"
	}
	return ""
}

// ConnectionURL DSN sin password (loopback con trust; ver diseño).
// El search_path viaja con ALTER ROLE ... IN DATABASE, no en la URL.
func ConnectionURL(host string, port int, db, role string) string {
	h := host
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	return "postgres://" + role + "@" + h + ":" + strconv.Itoa(port) + "/" + db + "?sslmode=disable"
}

// IsLoopbackHost true si host es loopback (el servidor solo escucha ahí).
func IsLoopbackHost(host string) bool {
	h := strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Provision plan completo para una app (sin tocar el servidor).
type Provision struct {
	Schema     string
	Role       string
	Mode       string // dedicated|shared
	DB         string
	URL        string
	Statements []string
}

// PlanProvision arma esquema/role/URL/statements para una app.
// mode "" = dedicated. db "" = DefaultDatabase.
func PlanProvision(app, mode, db, host string, port int) (Provision, error) {
	if mode == "" {
		mode = "dedicated"
	}
	if mode != "dedicated" && mode != "shared" {
		return Provision{}, fmt.Errorf("mode inválido %q (dedicated|shared)", mode)
	}
	if db == "" {
		db = DefaultDatabase
	}
	if err := ValidIdent(db); err != nil {
		return Provision{}, fmt.Errorf("base inválida: %w", err)
	}
	schema, err := SchemaForApp(app)
	if err != nil {
		return Provision{}, err
	}
	role := schema
	if mode == "shared" {
		role = SharedRole
	}
	stmts := []string{
		"CREATE SCHEMA IF NOT EXISTS " + QuoteIdent(schema),
	}
	if mode == "dedicated" {
		stmts = append(stmts,
			"CREATE ROLE "+QuoteIdent(role)+" WITH LOGIN",
			"GRANT USAGE, CREATE ON SCHEMA "+QuoteIdent(schema)+" TO "+QuoteIdent(role),
			"ALTER DEFAULT PRIVILEGES FOR ROLE "+QuoteIdent(role)+" IN SCHEMA "+QuoteIdent(schema)+
				" GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+QuoteIdent(role),
			"ALTER DEFAULT PRIVILEGES FOR ROLE "+QuoteIdent(role)+" IN SCHEMA "+QuoteIdent(schema)+
				" GRANT USAGE, SELECT ON SEQUENCES TO "+QuoteIdent(role),
			"ALTER ROLE "+QuoteIdent(role)+" IN DATABASE "+QuoteIdent(db)+" SET search_path = "+QuoteIdent(schema),
		)
	} else {
		// Compartido: el search_path global se fija aparte
		// (SharedSearchPathStmt) porque un rol cubre varios esquemas.
		stmts = append(stmts,
			"GRANT USAGE, CREATE ON SCHEMA "+QuoteIdent(schema)+" TO "+QuoteIdent(SharedRole),
			"ALTER DEFAULT PRIVILEGES FOR ROLE "+QuoteIdent(SharedRole)+" IN SCHEMA "+QuoteIdent(schema)+
				" GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+QuoteIdent(SharedRole),
			"ALTER DEFAULT PRIVILEGES FOR ROLE "+QuoteIdent(SharedRole)+" IN SCHEMA "+QuoteIdent(schema)+
				" GRANT USAGE, SELECT ON SEQUENCES TO "+QuoteIdent(SharedRole),
		)
	}
	return Provision{
		Schema: schema, Role: role, Mode: mode, DB: db,
		URL:        ConnectionURL(host, port, db, role),
		Statements: stmts,
	}, nil
}

// EnsureSharedRole crea el rol común idempotente (modo shared).
func EnsureSharedRole() string {
	return "CREATE ROLE " + QuoteIdent(SharedRole) + " WITH LOGIN"
}

// SharedSearchPathStmt fija el search_path global del rol compartido:
// todos sus esquemas + public. Un ALTER por (rol, db): cada provisión
// o borrado lo recalcula con la lista vigente.
func SharedSearchPathStmt(db string, schemas []string) string {
	quoted := make([]string, 0, len(schemas)+1)
	for _, s := range schemas {
		quoted = append(quoted, QuoteIdent(s))
	}
	quoted = append(quoted, `"public"`)
	return "ALTER ROLE " + QuoteIdent(SharedRole) + " IN DATABASE " + QuoteIdent(db) +
		" SET search_path = " + strings.Join(quoted, ", ")
}

// DropPlan desmonta un esquema: quita objetos del role y borra esquema.
// En modo shared conserva el rol común.
func DropPlan(schema, role, mode string) ([]string, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema inválido: %w", err)
	}
	if err := ValidIdent(role); err != nil {
		return nil, fmt.Errorf("rol inválido: %w", err)
	}
	if mode == "shared" {
		return []string{"DROP SCHEMA " + QuoteIdent(schema) + " CASCADE"}, nil
	}
	return []string{
		"REVOKE ALL ON SCHEMA " + QuoteIdent(schema) + " FROM " + QuoteIdent(role),
		"DROP OWNED BY " + QuoteIdent(role) + " CASCADE",
		"DROP SCHEMA " + QuoteIdent(schema) + " CASCADE",
		"DROP ROLE " + QuoteIdent(role),
	}, nil
}

// DumpArgs args de pg_dump -Fc por esquema (solo datos+def del esquema).
func DumpArgs(db, schema, out string) []string {
	return []string{"-Fc", "-f", out, "-n", schema, db}
}
