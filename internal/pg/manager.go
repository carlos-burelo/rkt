package pg

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// SuperDSN DSN del superusuario local. Sin sslmode: loopback de confianza
// tras las reglas hba que instala ApplyHBA (o scram con password).
func SuperDSN(host string, port int, user, pass, db string) string {
	h := host
	if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
		h = "[" + h + "]"
	}
	cred := user
	if pass != "" {
		cred = user + ":" + pass
	}
	return "postgres://" + cred + "@" + h + ":" + strconv.Itoa(port) + "/" + db + "?sslmode=disable&connect_timeout=10"
}

// Connect abre y verifica con timeout.
func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres no responde: %w", err)
	}
	return db, nil
}

// Version devuelve version() del servidor.
func Version(ctx context.Context, db *sql.DB) (string, error) {
	var v string
	if err := db.QueryRowContext(ctx, `SELECT version()`).Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}

// EnsureDatabase crea la DB si no existe (sin IF NOT EXISTS en PG).
func EnsureDatabase(ctx context.Context, db *sql.DB, name string) error {
	if err := ValidIdent(name); err != nil {
		return err
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `CREATE DATABASE `+QuoteIdent(name)); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	return nil
}

// roleExists true si el rol existe.
func roleExists(ctx context.Context, db *sql.DB, role string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = $1`, role).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ApplyProvision ejecuta un plan (idempotente: CREATE ROLE se salta si ya
// existe; el resto de sentencias lo son por construcción).
func ApplyProvision(ctx context.Context, db *sql.DB, p Provision) error {
	if p.Mode == "dedicated" {
		ok, err := roleExists(ctx, db, p.Role)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := db.ExecContext(ctx, `CREATE ROLE `+QuoteIdent(p.Role)+` WITH LOGIN`); err != nil {
				return fmt.Errorf("create role: %w", err)
			}
		}
	} else {
		ok, err := roleExists(ctx, db, SharedRole)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := db.ExecContext(ctx, EnsureSharedRole()); err != nil {
				return fmt.Errorf("create shared role: %w", err)
			}
		}
	}
	for _, st := range p.Statements {
		// CREATE ROLE ya resuelto arriba; el plan lo incluye por
		// completitud, aquí se salta para no romper la idempotencia.
		if strings.HasPrefix(st, "CREATE ROLE") {
			continue
		}
		if _, err := db.ExecContext(ctx, st); err != nil {
			return fmt.Errorf("provision: %w (%s)", err, st)
		}
	}
	return nil
}

// DropAll ejecuta un plan de borrado.
func DropAll(ctx context.Context, db *sql.DB, schema, role, mode string) error {
	stmts, err := DropPlan(schema, role, mode)
	if err != nil {
		return err
	}
	for _, st := range stmts {
		if _, err := db.ExecContext(ctx, st); err != nil {
			return fmt.Errorf("drop: %w (%s)", err, st)
		}
	}
	return nil
}

const (
	hbaBegin = "# --- rkt-managed begin (no editar: lo gestiona rkt pg) ---"
	hbaEnd   = "# --- rkt-managed end ---"
)

// ManagedHBA bloque de reglas trust para loopback (una app = su rol).
// Sin roles devuelve "" (sin bloque): no dejar marcadores huérfanos.
// Sin password en la URL: el acceso se limita por rol+DB+origen.
func ManagedHBA(db string, roles []string) string {
	seen := map[string]bool{}
	var rules []string
	for _, r := range roles {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		rules = append(rules, r)
	}
	if len(rules) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(hbaBegin + "\n")
	for _, r := range rules {
		b.WriteString("host    " + db + "    " + r + "    127.0.0.1/32            trust\n")
		b.WriteString("host    " + db + "    " + r + "    ::1/128                 trust\n")
	}
	b.WriteString(hbaEnd + "\n")
	return b.String()
}

// ApplyHBAFile escribe el bloque gestionado al inicio de pg_hba.conf
// (prioridad máxima: primera coincidencia manda). Guarda backup .rkt-bak
// la primera vez (y deja el archivo sin bloque si no hay reglas). Devuelve true si cambió algo.
func ApplyHBAFile(path, block string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(raw), "\n")
	kept := lines[:0:0]
	in := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == hbaBegin {
			in = true
			continue
		}
		if t == hbaEnd {
			in = false
			continue
		}
		if !in {
			kept = append(kept, l)
		}
	}
	next := block + strings.Join(kept, "\n")
	if next == string(raw) {
		return false, nil
	}
	if _, err := os.Stat(path + ".rkt-bak"); os.IsNotExist(err) {
		if err := os.WriteFile(path+".rkt-bak", raw, 0o600); err != nil {
			return false, err
		}
	}
	return true, os.WriteFile(path, []byte(next), 0o600)
}

// ReloadConf recarga pg_hba sin reiniciar.
func ReloadConf(ctx context.Context, db *sql.DB) error {
	var ok bool
	if err := db.QueryRowContext(ctx, `SELECT pg_reload_conf()`).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pg_reload_conf devolvió false")
	}
	return nil
}

// Tables lista tablas base del esquema (sin toast/temporales).
func Tables(ctx context.Context, db *sql.DB, schema string) ([]string, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname=$1 AND tablename NOT LIKE 'pg\_%' ESCAPE '\' ORDER BY tablename`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// cellValue convierte valores del driver a JSON serializable.
func cellValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(t)
	case string:
		return t
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	default:
		return v
	}
}

func scanRows(rows *sql.Rows) (cols []string, types []string, out [][]any, err error) {
	cols, err = rows.Columns()
	if err != nil {
		return nil, nil, nil, err
	}
	types = make([]string, len(cols))
	if cts, cerr := rows.ColumnTypes(); cerr == nil && len(cts) == len(cols) {
		for i, ct := range cts {
			types[i] = ct.DatabaseTypeName()
		}
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, nil, err
		}
		row := make([]any, len(cols))
		for i, v := range vals {
			row[i] = cellValue(v)
		}
		out = append(out, row)
	}
	return cols, types, out, rows.Err()
}

// ColInfo nombre y tipo de una columna (para cabeceras del visor).
type ColInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ColumnTypes columnas con su data_type (orden de la tabla).
func ColumnTypes(ctx context.Context, db *sql.DB, schema, table string) ([]ColInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, err
	}
	if err := ValidIdent(table); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ColInfo
	for rows.Next() {
		var c ColInfo
		if err := rows.Scan(&c.Name, &c.Type); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// TableRows pagina una tabla (limit capado) con su total.
// Sin filtros: atajo Supabase para el visor simple. Con filtros/orden usa TableRowsEx.
func TableRows(ctx context.Context, db *sql.DB, schema, table string, limit, offset int) (cols []string, rows [][]any, total int64, err error) {
	return TableRowsEx(ctx, db, schema, table, RowsQuery{Limit: limit, Offset: offset})
}

// textColsLive columnas de texto para la búsqueda global (máx 10).
func textColsLive(ctx context.Context, db *sql.DB, schema, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND data_type IN ('text','character varying','character','uuid','json','jsonb','inet') ORDER BY ordinal_position LIMIT 10`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// TableRowsEx paginado Supabase: filtros AND, búsqueda global OR, orden y
// total filtrado (no el total de la tabla).
func TableRowsEx(ctx context.Context, db *sql.DB, schema, table string, q RowsQuery) (cols []string, rows [][]any, total int64, err error) {
	if err := ValidIdent(schema); err != nil {
		return nil, nil, 0, err
	}
	if err := ValidIdent(table); err != nil {
		return nil, nil, 0, err
	}
	q = normalizeRowsQuery(q)
	var searchCols []string
	if strings.TrimSpace(q.Search) != "" {
		searchCols, err = textColsLive(ctx, db, schema, table)
		if err != nil {
			return nil, nil, 0, err
		}
	}
	sel, args, err := BuildSelect(schema, table, q, searchCols)
	if err != nil {
		return nil, nil, 0, err
	}
	rs, err := db.QueryContext(ctx, sel, args...)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rs.Close()
	cols, _, rows, err = scanRows(rs)
	if err != nil {
		return nil, nil, 0, err
	}
	cnt, cargs, err := BuildCount(schema, table, q.Filters, q.Search, searchCols)
	if err != nil {
		return nil, nil, 0, err
	}
	if err := db.QueryRowContext(ctx, cnt, cargs...).Scan(&total); err != nil {
		return nil, nil, 0, err
	}
	if cols == nil {
		cols = []string{}
	}
	if rows == nil {
		rows = [][]any{}
	}
	return cols, rows, total, nil
}

// DescribeTable inspector de tabla: columnas, PK, RLS y conteo exacto.
func DescribeTable(ctx context.Context, db *sql.DB, schema, table string) (TableSchema, error) {
	var out TableSchema
	if err := ValidIdent(schema); err != nil {
		return out, err
	}
	if err := ValidIdent(table); err != nil {
		return out, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 ORDER BY ordinal_position`,
		schema, table)
	if err != nil {
		return out, err
	}
	type raw struct {
		name, typ, nullable string
		def                 sql.NullString
	}
	var raws []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.name, &r.typ, &r.nullable, &r.def); err != nil {
			rows.Close()
			return out, err
		}
		raws = append(raws, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(raws) == 0 {
		return out, fmt.Errorf("tabla %q inexistente", table)
	}
	pkRows, err := db.QueryContext(ctx,
		`SELECT a.attname FROM pg_index i JOIN pg_class c ON c.oid=i.indrelid JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum=ANY(i.indkey) WHERE n.nspname=$1 AND c.relname=$2 AND i.indisprimary ORDER BY a.attnum`,
		schema, table)
	if err != nil {
		return out, err
	}
	pkSet := map[string]bool{}
	var pk []string
	for pkRows.Next() {
		var n string
		if err := pkRows.Scan(&n); err != nil {
			pkRows.Close()
			return out, err
		}
		pkSet[n] = true
		pk = append(pk, n)
	}
	pkRows.Close()
	if err := pkRows.Err(); err != nil {
		return out, err
	}
	var rls, forced bool
	if err := db.QueryRowContext(ctx,
		`SELECT c.relrowsecurity, c.relforcerowsecurity FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`,
		schema, table).Scan(&rls, &forced); err != nil {
		return out, err
	}
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+QuoteIdent(schema)+`.`+QuoteIdent(table)).Scan(&total); err != nil {
		return out, err
	}
	cols := make([]ColumnDetail, 0, len(raws))
	for _, r := range raws {
		c := ColumnDetail{Name: r.name, Type: r.typ, Nullable: r.nullable == "YES", IsPK: pkSet[r.name]}
		if r.def.Valid {
			d := r.def.String
			c.Default = &d
		}
		cols = append(cols, c)
	}
	if pk == nil {
		pk = []string{}
	}
	return TableSchema{Table: table, Columns: cols, PK: pk, RLS: rls, RLSForce: forced, Rows: total}, nil
}

// TableCounts filas exactas por tabla (para la lista del Studio).
func TableCounts(ctx context.Context, db *sql.DB, schema string, tables []string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range tables {
		if err := ValidIdent(t); err != nil {
			return nil, err
		}
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+QuoteIdent(schema)+`.`+QuoteIdent(t)).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}

// InsertRow inserta y devuelve la fila creada (RETURNING * como mapa).
func InsertRow(ctx context.Context, db *sql.DB, schema, table string, data map[string]any) (map[string]any, error) {
	q, args, err := BuildInsert(schema, table, data)
	if err != nil {
		return nil, err
	}
	rs, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, _, rows, err := scanRows(rs)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(cols))
	for i, c := range cols {
		out[c] = rows[0][i]
	}
	return out, nil
}

// UpdateRow actualiza por PK exacta; devuelve filas afectadas.
func UpdateRow(ctx context.Context, db *sql.DB, schema, table string, pk, patch map[string]any) (int64, error) {
	q, args, err := BuildUpdate(schema, table, pk, patch)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteRows borra por PK exacta o por filtros; devuelve filas afectadas.
func DeleteRows(ctx context.Context, db *sql.DB, schema, table string, pk map[string]any, filters []RowFilter) (int64, error) {
	q, args, err := BuildDelete(schema, table, pk, filters)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CreateTableExec crea una tabla desde el Studio (columnas validadas).
func CreateTableExec(ctx context.Context, db *sql.DB, schema, table string, cols []ColumnDef) error {
	q, err := BuildCreateTable(schema, table, cols)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// AddColumnExec añade una columna validada.
func AddColumnExec(ctx context.Context, db *sql.DB, schema, table string, col ColumnDef) error {
	q, err := BuildAddColumn(schema, table, col)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// DropColumnExec quita una columna (con confirmación en capas superiores).
func DropColumnExec(ctx context.Context, db *sql.DB, schema, table, col string) error {
	if err := ValidIdent(schema); err != nil {
		return err
	}
	if err := ValidIdent(table); err != nil {
		return err
	}
	if err := ValidIdent(col); err != nil {
		return fmt.Errorf("col: %w", err)
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE `+QuoteIdent(schema)+`.`+QuoteIdent(table)+` DROP COLUMN `+QuoteIdent(col))
	return err
}

// DropTableExec borra una tabla del esquema (con confirmación arriba).
func DropTableExec(ctx context.Context, db *sql.DB, schema, table string) error {
	if err := ValidIdent(schema); err != nil {
		return err
	}
	if err := ValidIdent(table); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `DROP TABLE `+QuoteIdent(schema)+`.`+QuoteIdent(table))
	return err
}

// SetRLS activa o desactiva row level security en una tabla.
func SetRLS(ctx context.Context, db *sql.DB, schema, table string, enable bool) error {
	if err := ValidIdent(schema); err != nil {
		return err
	}
	if err := ValidIdent(table); err != nil {
		return err
	}
	verb := `DISABLE`
	if enable {
		verb = `ENABLE`
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE `+QuoteIdent(schema)+`.`+QuoteIdent(table)+` `+verb+` ROW LEVEL SECURITY`)
	return err
}

// Policy describe una policy RLS (para el inspector).
type Policy struct {
	Name    string `json:"name"`
	Command string `json:"cmd"`
	Roles   string `json:"roles"`
	Using   string `json:"using,omitempty"`
	Check   string `json:"check,omitempty"`
}

// ListPolicies policies RLS de una tabla (vacío = sin policies).
func ListPolicies(ctx context.Context, db *sql.DB, schema, table string) ([]Policy, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, err
	}
	if err := ValidIdent(table); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT policyname, cmd, roles, qual, with_check FROM pg_policies WHERE schemaname=$1 AND tablename=$2 ORDER BY policyname`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Policy{}
	for rows.Next() {
		var p Policy
		var qual, check sql.NullString
		var roles string
		_ = roles
		if err := rows.Scan(&p.Name, &p.Command, &roles, &qual, &check); err != nil {
			return nil, err
		}
		p.Roles = roles
		if qual.Valid {
			p.Using = qual.String
		}
		if check.Valid {
			p.Check = check.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// QueryResult resultado de consola SQL.
type QueryResult struct {
	Columns  []string `json:"columns,omitempty"`
	Types    []string `json:"types,omitempty"`
	Rows     [][]any  `json:"rows,omitempty"`
	Affected int64    `json:"affected,omitempty"`
	Command  string   `json:"command"`
}

// isReadOnly true para consultas que no mutan.
func isReadOnly(sqlText string) bool {
	w := strings.ToUpper(strings.TrimSpace(sqlText))
	for _, p := range []string{"SELECT", "WITH", "TABLE", "VALUES", "EXPLAIN", "SHOW", "DESCRIBE"} {
		if strings.HasPrefix(w, p+" ") || strings.HasPrefix(w, p+"(") || w == p {
			return true
		}
	}
	return false
}

// Query ejecuta SQL arbitrario en la DB del rol. SELECT-like devuelve filas;
// el resto, filas afectadas. Una sola sentencia por ejecución.
func Query(ctx context.Context, db *sql.DB, sqlText string) (QueryResult, error) {
	t := strings.TrimSpace(sqlText)
	if t == "" {
		return QueryResult{}, fmt.Errorf("sql vacío")
	}
	nonEmpty := 0
	for _, p := range strings.Split(t, ";") {
		if strings.TrimSpace(p) != "" {
			nonEmpty++
		}
	}
	if nonEmpty > 1 {
		return QueryResult{}, fmt.Errorf("una sola sentencia por ejecución")
	}
	if isReadOnly(t) {
		rs, err := db.QueryContext(ctx, t)
		if err != nil {
			return QueryResult{}, err
		}
		defer rs.Close()
		cols, types, rows, err := scanRows(rs)
		if err != nil {
			return QueryResult{}, err
		}
		return QueryResult{Columns: cols, Types: types, Rows: rows, Command: "SELECT"}, nil
	}
	res, err := db.ExecContext(ctx, t)
	if err != nil {
		return QueryResult{}, err
	}
	n, _ := res.RowsAffected()
	return QueryResult{Affected: n, Command: "EXEC"}, nil
}

// Restore vuelca un backup custom (-Fc) opcionalmente limitado a un esquema.
func Restore(ctx context.Context, pgRestore, host string, port int, user, pass, db, file string) error {
	if _, err := os.Stat(file); err != nil {
		return fmt.Errorf("backup no encontrado: %s", file)
	}
	if pgRestore == "" {
		return fmt.Errorf("pg_restore no disponible")
	}
	cmd := exec.CommandContext(ctx, pgRestore,
		"-h", host, "-p", strconv.Itoa(port), "-U", user,
		"-d", db, "--clean", "--if-exists", file)
	if pass != "" {
		cmd.Env = append(os.Environ(), "PGPASSWORD="+pass)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_restore: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BackupTo vuelca un esquema en formato custom (-Fc) con pg_dump.
// PGPASSWORD via entorno del proceso hijo (no va en argv).
func BackupTo(ctx context.Context, pgDumpBin, host string, port int, user, pass, db, schema, out string) error {
	if pgDumpBin == "" {
		return fmt.Errorf("pg_dump no disponible")
	}
	if err := ValidIdent(schema); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, pgDumpBin,
		"-h", host, "-p", strconv.Itoa(port), "-U", user,
		"-Fc", "-f", out, "-n", schema, db)
	if pass != "" {
		cmd.Env = append(os.Environ(), "PGPASSWORD="+pass)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
