package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// IndexInfo índice de una tabla (para el inspector del Studio).
type IndexInfo struct {
	Name    string `json:"name"`
	Table   string `json:"table"`
	Def     string `json:"def"`
	Unique  bool   `json:"unique"`
	Primary bool   `json:"primary"`
}

// ListIndexes índices del esquema (todas o una tabla). Orden por nombre.
func ListIndexes(ctx context.Context, db *sql.DB, schema, table string) ([]IndexInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	if table != "" {
		if err := ValidIdent(table); err != nil {
			return nil, fmt.Errorf("tabla: %w", err)
		}
	}
	q := `SELECT i.indexname, i.tablename, i.indexdef, COALESCE(ix.indisprimary,false), COALESCE(ix.indisunique,false)
		FROM pg_indexes i
		JOIN pg_class c ON c.relname=i.indexname
		JOIN pg_namespace n ON n.oid=c.relnamespace AND n.nspname=i.schemaname
		LEFT JOIN pg_index ix ON ix.indexrelid=c.oid
		WHERE i.schemaname=$1`
	args := []any{schema}
	if table != "" {
		q += ` AND i.tablename=$2`
		args = append(args, table)
	}
	q += ` ORDER BY i.indexname`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IndexInfo{}
	for rows.Next() {
		var in IndexInfo
		if err := rows.Scan(&in.Name, &in.Table, &in.Def, &in.Primary, &in.Unique); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// indexMethods métodos permitidos (lista cerrada: lo demás va por SQL).
var indexMethods = map[string]bool{"btree": true, "hash": true, "gin": true, "gist": true, "brin": true}

// BuildCreateIndex CREATE [UNIQUE] INDEX (nombre opcional: tabla_columnas_idx).
func BuildCreateIndex(schema, table, name string, columns []string, unique bool, method string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if len(columns) == 0 || len(columns) > 16 {
		return "", fmt.Errorf("columnas 1..16")
	}
	for _, c := range columns {
		if err := ValidIdent(c); err != nil {
			return "", fmt.Errorf("col %q: %w", c, err)
		}
	}
	m := strings.ToLower(strings.TrimSpace(method))
	if m == "" {
		m = "btree"
	}
	if !indexMethods[m] {
		return "", fmt.Errorf("método %q (btree|hash|gin|gist|brin)", method)
	}
	n := strings.TrimSpace(name)
	if n == "" {
		n = table + "_" + strings.Join(columns, "_") + "_idx"
		if len(n) > 60 {
			n = n[:60]
		}
	}
	if err := ValidIdent(strings.ToLower(n)); err != nil {
		return "", fmt.Errorf("nombre: %w", err)
	}
	n = strings.ToLower(n)
	cols := make([]string, 0, len(columns))
	for _, c := range columns {
		cols = append(cols, QuoteIdent(c))
	}
	uniq := ""
	if unique {
		uniq = "UNIQUE "
	}
	return fmt.Sprintf(`CREATE %sINDEX %s ON %s.%s USING %s (%s)`,
		uniq, QuoteIdent(n), QuoteIdent(schema), QuoteIdent(table), m, strings.Join(cols, ", ")), nil
}

// BuildDropIndex DROP INDEX del esquema.
func BuildDropIndex(schema, name string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(strings.ToLower(strings.TrimSpace(name))); err != nil {
		return "", fmt.Errorf("índice: %w", err)
	}
	return `DROP INDEX ` + QuoteIdent(schema) + `.` + QuoteIdent(strings.ToLower(strings.TrimSpace(name))), nil
}

// CreateIndexExec crea un índice validado.
func CreateIndexExec(ctx context.Context, db *sql.DB, schema, table, name string, columns []string, unique bool, method string) error {
	q, err := BuildCreateIndex(schema, table, name, columns, unique, method)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// DropIndexExec borra un índice del esquema.
func DropIndexExec(ctx context.Context, db *sql.DB, schema, name string) error {
	q, err := BuildDropIndex(schema, name)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// ConstraintInfo restricción (PK/UNIQUE/FK/CHECK) para el inspector.
type ConstraintInfo struct {
	Name      string `json:"name"`
	Table     string `json:"table"`
	Kind      string `json:"kind"`
	Def       string `json:"def"`
	Columns   string `json:"columns,omitempty"`
	RefTable  string `json:"ref_table,omitempty"`
	RefColumn string `json:"ref_columns,omitempty"`
}

// ListConstraints restricciones del esquema (todas o una tabla).
func ListConstraints(ctx context.Context, db *sql.DB, schema, table string) ([]ConstraintInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	if table != "" {
		if err := ValidIdent(table); err != nil {
			return nil, fmt.Errorf("tabla: %w", err)
		}
	}
	q := `SELECT con.conname, rel.relname, con.contype, pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid=con.conrelid
		JOIN pg_namespace n ON n.oid=rel.relnamespace
		WHERE n.nspname=$1`
	args := []any{schema}
	if table != "" {
		q += ` AND rel.relname=$2`
		args = append(args, table)
	}
	q += ` ORDER BY rel.relname, con.conname`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	kindName := map[string]string{"p": "primary", "u": "unique", "f": "foreign", "c": "check", "t": "trigger", "x": "exclusion"}
	out := []ConstraintInfo{}
	for rows.Next() {
		var name, tbl, ctype, def string
		if err := rows.Scan(&name, &tbl, &ctype, &def); err != nil {
			return nil, err
		}
		kind := kindName[ctype]
		if kind == "" {
			kind = ctype
		}
		out = append(out, ConstraintInfo{Name: name, Table: tbl, Kind: kind, Def: def})
	}
	return out, rows.Err()
}
