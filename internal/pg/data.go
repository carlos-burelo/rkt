// Tablas y filas estilo Supabase: construcción pura de SQL (sin tocar el
// servidor). Todo valida identificadores y tipos antes de interpolar; los
// valores viajan siempre como parámetros $n. Testeable sin Postgres.
package pg

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ColumnDetail describe una columna para el inspector del Studio.
type ColumnDetail struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default,omitempty"`
	IsPK     bool    `json:"is_pk"`
}

// TableSchema inspector de una tabla: columnas, PK, RLS y conteo.
type TableSchema struct {
	Table    string         `json:"table"`
	Columns  []ColumnDetail `json:"columns"`
	PK       []string       `json:"pk"`
	RLS      bool           `json:"rls"`
	RLSForce bool           `json:"rls_force"`
	Rows     int64          `json:"rows"`
}

// RowFilter filtro AND del visor: col:op:valor (valor con : permitido).
// Ops: eq neq gt gte lt lte like ilike in isnull isnotnull.
type RowFilter struct {
	Col   string `json:"col"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// RowsQuery paginado + orden + filtros + búsqueda del visor.
type RowsQuery struct {
	Limit    int
	Offset   int
	OrderCol string
	OrderDir string // asc|desc (vacío = sin orden)
	Filters  []RowFilter
	Search   string // ilike %q% sobre columnas de texto (OR)
}

var filterOps = map[string]bool{
	"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true,
	"like": true, "ilike": true, "in": true, "isnull": true, "isnotnull": true,
}

// ParseFilter parte "col:op:valor" (el valor puede traer :).
func ParseFilter(s string) (RowFilter, error) {
	i := strings.Index(s, ":")
	if i < 0 {
		return RowFilter{}, fmt.Errorf("filtro %q: usa col:op:valor", s)
	}
	j := strings.Index(s[i+1:], ":")
	if j < 0 {
		return RowFilter{}, fmt.Errorf("filtro %q: usa col:op:valor", s)
	}
	f := RowFilter{Col: s[:i], Op: s[i+1 : i+1+j], Value: s[i+1+j+1:]}
	if err := ValidIdent(f.Col); err != nil {
		return RowFilter{}, fmt.Errorf("filtro col: %w", err)
	}
	if !filterOps[f.Op] {
		return RowFilter{}, fmt.Errorf("filtro op %q (eq|neq|gt|gte|lt|lte|like|ilike|in|isnull|isnotnull)", f.Op)
	}
	if (f.Op == "isnull" || f.Op == "isnotnull") && f.Value != "" {
		return RowFilter{}, fmt.Errorf("filtro %q no lleva valor", f.Op)
	}
	if f.Op == "in" {
		parts := splitList(f.Value)
		if len(parts) == 0 {
			return RowFilter{}, fmt.Errorf("filtro in vacío")
		}
		if len(parts) > 100 {
			return RowFilter{}, fmt.Errorf("filtro in: máx 100 valores")
		}
	}
	return f, nil
}

// ParseFilters parte una lista (máx 20) y falla en la primera inválida.
func ParseFilters(in []string) ([]RowFilter, error) {
	if len(in) > 20 {
		return nil, fmt.Errorf("máx 20 filtros")
	}
	out := make([]RowFilter, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		f, err := ParseFilter(s)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// whereClause arma el WHERE (sin la palabra) con placeholders desde $base.
// Devuelve cláusula, args y siguiente índice libre.
func whereClause(filters []RowFilter, searchCols []string, search string, base int) (string, []any, int, error) {
	var parts []string
	var args []any
	next := base
	for _, f := range filters {
		c := QuoteIdent(f.Col)
		switch f.Op {
		case "eq":
			parts = append(parts, c+" = $"+strconv.Itoa(next))
			args = append(args, f.Value)
			next++
		case "neq":
			parts = append(parts, c+" <> $"+strconv.Itoa(next))
			args = append(args, f.Value)
			next++
		case "gt", "gte", "lt", "lte":
			op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[f.Op]
			parts = append(parts, c+" "+op+" $"+strconv.Itoa(next))
			args = append(args, f.Value)
			next++
		case "like":
			parts = append(parts, c+" LIKE $"+strconv.Itoa(next))
			args = append(args, f.Value)
			next++
		case "ilike":
			parts = append(parts, c+" ILIKE $"+strconv.Itoa(next))
			args = append(args, f.Value)
			next++
		case "in":
			vals := splitList(f.Value)
			holders := make([]string, 0, len(vals))
			for _, v := range vals {
				holders = append(holders, "$"+strconv.Itoa(next))
				args = append(args, v)
				next++
			}
			parts = append(parts, c+" IN ("+strings.Join(holders, ", ")+")")
		case "isnull":
			parts = append(parts, c+" IS NULL")
		case "isnotnull":
			parts = append(parts, c+" IS NOT NULL")
		}
	}
	if strings.TrimSpace(search) != "" && len(searchCols) > 0 {
		ors := make([]string, 0, len(searchCols))
		for _, col := range searchCols {
			ors = append(ors, QuoteIdent(col)+"::text ILIKE $"+strconv.Itoa(next))
		}
		args = append(args, "%"+strings.TrimSpace(search)+"%")
		next++
		parts = append(parts, "("+strings.Join(ors, " OR ")+")")
	}
	return strings.Join(parts, " AND "), args, next, nil
}

// orderClause valida columna+dirección (vacío = sin orden).
func orderClause(col, dir string) (string, error) {
	if col == "" {
		return "", nil
	}
	if err := ValidIdent(col); err != nil {
		return "", fmt.Errorf("orden col: %w", err)
	}
	d := strings.ToLower(strings.TrimSpace(dir))
	if d == "" {
		d = "asc"
	}
	if d != "asc" && d != "desc" {
		return "", fmt.Errorf("orden dir %q (asc|desc)", dir)
	}
	return `ORDER BY ` + QuoteIdent(col) + ` ` + strings.ToUpper(d), nil
}

// normalizeRowsQuery aplica topes Supabase-like: limit 1..500, offset >= 0.
func normalizeRowsQuery(q RowsQuery) RowsQuery {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return q
}

// textSearchCols columnas candidatas para búsqueda: las de texto de la tabla.
func textSearchCols(cols []ColumnDetail) []string {
	var out []string
	for _, c := range cols {
		t := strings.ToLower(c.Type)
		if strings.Contains(t, "char") || t == "text" || t == "uuid" || t == "json" || t == "jsonb" || t == "inet" {
			out = append(out, c.Name)
		}
		if len(out) >= 10 {
			break
		}
	}
	return out
}

// BuildSelect SELECT paginado con filtros/orden/búsqueda. searchCols son las
// columnas de texto (vacío = sin búsqueda). Devuelve SQL + args.
func BuildSelect(schema, table string, q RowsQuery, searchCols []string) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	q = normalizeRowsQuery(q)
	where, args, next, err := whereClause(q.Filters, searchCols, q.Search, 1)
	if err != nil {
		return "", nil, err
	}
	ord, err := orderClause(q.OrderCol, q.OrderDir)
	if err != nil {
		return "", nil, err
	}
	var b strings.Builder
	b.WriteString(`SELECT * FROM ` + QuoteIdent(schema) + `.` + QuoteIdent(table))
	if where != "" {
		b.WriteString(` WHERE ` + where)
	}
	if ord != "" {
		b.WriteString(` ` + ord)
	}
	b.WriteString(` LIMIT $` + strconv.Itoa(next) + ` OFFSET $` + strconv.Itoa(next+1))
	args = append(args, q.Limit, q.Offset)
	return b.String(), args, nil
}

// BuildCount COUNT(*) con los mismos filtros/búsqueda (total filtrado).
func BuildCount(schema, table string, filters []RowFilter, search string, searchCols []string) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	where, args, _, err := whereClause(filters, searchCols, search, 1)
	if err != nil {
		return "", nil, err
	}
	q := `SELECT count(*) FROM ` + QuoteIdent(schema) + `.` + QuoteIdent(table)
	if where != "" {
		q += ` WHERE ` + where
	}
	return q, args, nil
}

// BuildInsert INSERT con RETURNING * (data no vacío, claves válidas).
func BuildInsert(schema, table string, data map[string]any) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("data vacío")
	}
	if len(data) > 100 {
		return "", nil, fmt.Errorf("máx 100 columnas por fila")
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		if err := ValidIdent(k); err != nil {
			return "", nil, fmt.Errorf("col %q: %w", k, err)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cols := make([]string, 0, len(keys))
	holders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for i, k := range keys {
		cols = append(cols, QuoteIdent(k))
		holders = append(holders, "$"+strconv.Itoa(i+1))
		args = append(args, data[k])
	}
	q := `INSERT INTO ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` (` + strings.Join(cols, ", ") + `) VALUES (` + strings.Join(holders, ", ") + `) RETURNING *`
	return q, args, nil
}

// BuildUpdate UPDATE por PK exacta (pk y patch no vacíos).
func BuildUpdate(schema, table string, pk, patch map[string]any) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	if len(pk) == 0 {
		return "", nil, fmt.Errorf("pk requerido (sin WHERE completo no se actualiza)")
	}
	if len(patch) == 0 {
		return "", nil, fmt.Errorf("patch vacío")
	}
	if len(pk)+len(patch) > 100 {
		return "", nil, fmt.Errorf("máx 100 columnas")
	}
	setKeys := make([]string, 0, len(patch))
	for k := range patch {
		if err := ValidIdent(k); err != nil {
			return "", nil, fmt.Errorf("col %q: %w", k, err)
		}
		setKeys = append(setKeys, k)
	}
	sort.Strings(setKeys)
	whereKeys := make([]string, 0, len(pk))
	for k := range pk {
		if err := ValidIdent(k); err != nil {
			return "", nil, fmt.Errorf("pk %q: %w", k, err)
		}
		whereKeys = append(whereKeys, k)
	}
	sort.Strings(whereKeys)
	var sets []string
	var args []any
	next := 1
	for _, k := range setKeys {
		sets = append(sets, QuoteIdent(k)+" = $"+strconv.Itoa(next))
		args = append(args, patch[k])
		next++
	}
	var conds []string
	for _, k := range whereKeys {
		conds = append(conds, QuoteIdent(k)+" = $"+strconv.Itoa(next))
		args = append(args, pk[k])
		next++
	}
	q := `UPDATE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` SET ` + strings.Join(sets, ", ") + ` WHERE ` + strings.Join(conds, " AND ")
	return q, args, nil
}

// BuildDelete DELETE por PK exacta o por filtros (uno de los dos obligatorio).
func BuildDelete(schema, table string, pk map[string]any, filters []RowFilter) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	if len(pk) == 0 && len(filters) == 0 {
		return "", nil, fmt.Errorf("pk o filtros requeridos (sin WHERE no se borra)")
	}
	if len(pk) > 0 && len(filters) > 0 {
		return "", nil, fmt.Errorf("pk o filtros, no ambos")
	}
	if len(pk) > 0 {
		keys := make([]string, 0, len(pk))
		for k := range pk {
			if err := ValidIdent(k); err != nil {
				return "", nil, fmt.Errorf("pk %q: %w", k, err)
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var conds []string
		var args []any
		for i, k := range keys {
			conds = append(conds, QuoteIdent(k)+" = $"+strconv.Itoa(i+1))
			args = append(args, pk[k])
		}
		return `DELETE FROM ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
			` WHERE ` + strings.Join(conds, " AND "), args, nil
	}
	where, args, _, err := whereClause(filters, nil, "", 1)
	if err != nil {
		return "", nil, err
	}
	return `DELETE FROM ` + QuoteIdent(schema) + `.` + QuoteIdent(table) + ` WHERE ` + where, args, nil
}

// ColumnDef definición de columna para crear tablas desde el Studio.
type ColumnDef struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	NotNull bool   `json:"not_null,omitempty"`
	Default string `json:"default,omitempty"`
	PK      bool   `json:"pk,omitempty"`
}

var colTypeRe = regexp.MustCompile(`^[a-z]+( precision)?(\s+(with|without)\s+time\s+zone)?(\(\d+(,\d+)?\))?$`)

// ValidColType tipos permitidos (lista cerrada estilo Supabase: sin enums ni
// dominios arbitrarios; eso se hace con SQL).
func ValidColType(t string) error {
	norm := strings.ToLower(strings.TrimSpace(t))
	allowed := map[string]bool{
		"text": true, "boolean": true, "bool": true,
		"smallint": true, "integer": true, "int": true, "bigint": true,
		"real": true, "double precision": true, "numeric": true,
		"varchar": true, "character varying": true, "char": true, "character": true,
		"uuid": true, "jsonb": true, "json": true, "bytea": true,
		"timestamptz": true, "timestamp": true,
		"timestamp with time zone": true, "timestamp without time zone": true,
		"date": true, "time": true, "inet": true, "interval": true,
	}
	base := norm
	if i := strings.Index(base, "("); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	if !allowed[base] && !allowed[norm] {
		return fmt.Errorf("tipo %q no permitido (text|int|bigint|bool|numeric|varchar(n)|uuid|jsonb|timestamptz|date|…)", t)
	}
	if !colTypeRe.MatchString(norm) {
		return fmt.Errorf("tipo %q inválido", t)
	}
	// numeric/varchar con precisión: cotas sanas.
	if i := strings.Index(norm, "("); i >= 0 {
		inner := norm[i+1 : len(norm)-1]
		for _, p := range strings.Split(inner, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 1 || n > 10000 {
				return fmt.Errorf("precisión %q fuera de rango", p)
			}
		}
	}
	return nil
}

var colDefaultRe = regexp.MustCompile(`^(?i)(NULL|TRUE|FALSE|NOW\(\)|CURRENT_TIMESTAMP|CURRENT_DATE|CURRENT_TIME|GEN_RANDOM_UUID\(\)|UUID_GENERATE_V4\(\)|-?\d+(\.\d+)?|'([^']|'')*')$`)

// ValidColDefault defaults cerrados (literales, now(), gen_random_uuid()).
func ValidColDefault(d string) error {
	if strings.TrimSpace(d) == "" {
		return nil
	}
	if !colDefaultRe.MatchString(strings.TrimSpace(d)) {
		return fmt.Errorf("default %q no permitido (literales, now(), gen_random_uuid())", d)
	}
	return nil
}

// BuildCreateTable CREATE TABLE con columnas validadas (si no hay PK, sin PK).
func BuildCreateTable(schema, table string, cols []ColumnDef) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if len(cols) == 0 || len(cols) > 100 {
		return "", fmt.Errorf("columnas 1..100")
	}
	seen := map[string]bool{}
	var defs []string
	var pks []string
	for _, c := range cols {
		if err := ValidIdent(c.Name); err != nil {
			return "", fmt.Errorf("col %q: %w", c.Name, err)
		}
		if seen[c.Name] {
			return "", fmt.Errorf("col %q duplicada", c.Name)
		}
		seen[c.Name] = true
		if err := ValidColType(c.Type); err != nil {
			return "", err
		}
		if err := ValidColDefault(c.Default); err != nil {
			return "", err
		}
		d := QuoteIdent(c.Name) + ` ` + strings.ToLower(strings.TrimSpace(c.Type))
		if c.NotNull || c.PK {
			d += ` NOT NULL`
		}
		if strings.TrimSpace(c.Default) != "" {
			d += ` DEFAULT ` + strings.TrimSpace(c.Default)
		}
		defs = append(defs, d)
		if c.PK {
			pks = append(pks, QuoteIdent(c.Name))
		}
	}
	if len(pks) > 0 {
		defs = append(defs, `PRIMARY KEY (`+strings.Join(pks, ", ")+`)`)
	}
	return `CREATE TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		" (\n  " + strings.Join(defs, ",\n  ") + "\n)", nil
}

// BuildAddColumn ALTER TABLE ADD COLUMN validado.
func BuildAddColumn(schema, table string, col ColumnDef) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(col.Name); err != nil {
		return "", fmt.Errorf("col: %w", err)
	}
	if err := ValidColType(col.Type); err != nil {
		return "", err
	}
	if err := ValidColDefault(col.Default); err != nil {
		return "", err
	}
	if col.PK {
		return "", fmt.Errorf("pk en ADD COLUMN no: crea la tabla con pk o usa SQL")
	}
	d := `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` ADD COLUMN ` + QuoteIdent(col.Name) + ` ` + strings.ToLower(strings.TrimSpace(col.Type))
	if col.NotNull {
		d += ` NOT NULL`
	}
	if strings.TrimSpace(col.Default) != "" {
		d += ` DEFAULT ` + strings.TrimSpace(col.Default)
	}
	return d, nil
}
