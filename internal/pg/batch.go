package pg

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// BuildInsertMany INSERT múltiple con RETURNING ausente (devuelve n).
// Columnas fijas, filas 1..1000, celdas totales ≤ 10000.
func BuildInsertMany(schema, table string, columns []string, rows [][]any) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	if len(columns) == 0 || len(columns) > 100 {
		return "", nil, fmt.Errorf("columnas 1..100")
	}
	seen := map[string]bool{}
	for _, c := range columns {
		if err := ValidIdent(c); err != nil {
			return "", nil, fmt.Errorf("col %q: %w", c, err)
		}
		if seen[c] {
			return "", nil, fmt.Errorf("col %q duplicada", c)
		}
		seen[c] = true
	}
	if len(rows) == 0 || len(rows) > 1000 {
		return "", nil, fmt.Errorf("filas 1..1000")
	}
	if len(columns)*len(rows) > 20000 {
		return "", nil, fmt.Errorf("demasiadas celdas (máx 20000)")
	}
	for i, r := range rows {
		if len(r) != len(columns) {
			return "", nil, fmt.Errorf("fila %d: %d valores, se esperaban %d", i+1, len(r), len(columns))
		}
	}
	cols := make([]string, 0, len(columns))
	for _, c := range columns {
		cols = append(cols, QuoteIdent(c))
	}
	var b strings.Builder
	b.WriteString(`INSERT INTO ` + QuoteIdent(schema) + `.` + QuoteIdent(table) + ` (` + strings.Join(cols, ", ") + `) VALUES `)
	args := make([]any, 0, len(columns)*len(rows))
	next := 1
	tuples := make([]string, 0, len(rows))
	for _, r := range rows {
		holders := make([]string, 0, len(columns))
		for _, v := range r {
			holders = append(holders, fmt.Sprintf("$%d", next))
			args = append(args, v)
			next++
		}
		tuples = append(tuples, "("+strings.Join(holders, ", ")+")")
	}
	b.WriteString(strings.Join(tuples, ", "))
	return b.String(), args, nil
}

// InsertManyExec inserta un lote y devuelve filas afectadas.
func InsertManyExec(ctx context.Context, db *sql.DB, schema, table string, columns []string, rows [][]any) (int64, error) {
	q, args, err := BuildInsertMany(schema, table, columns, rows)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ExportQuery SELECT * con mismos filtros que el visor pero tope de
// exportación (10k) y sin paginación: orden + filtros + búsqueda.
func ExportQuery(schema, table string, q RowsQuery, searchCols []string) (string, []any, error) {
	if err := ValidIdent(schema); err != nil {
		return "", nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", nil, fmt.Errorf("tabla: %w", err)
	}
	q.Limit = 10000
	q.Offset = 0
	return BuildSelect(schema, table, q, searchCols)
}

// SplitStatements parte SQL en sentencias respetando '...', "..." y
// comentarios -- y /* */. Sin punto y coma final por sentencia.
func SplitStatements(src string) []string {
	var out []string
	var cur strings.Builder
	var inStr byte // 0, '\'' o '"'
	lineComment, blockComment := false, false
	flush := func() {
		if t := strings.TrimSpace(cur.String()); t != "" {
			out = append(out, strings.TrimSuffix(strings.TrimSpace(cur.String()), ";"))
		}
		cur.Reset()
	}
	runes := src
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		var nxt byte
		if i+1 < len(runes) {
			nxt = runes[i+1]
		}
		if lineComment {
			cur.WriteByte(c)
			if c == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			cur.WriteByte(c)
			if c == '*' && nxt == '/' {
				cur.WriteByte(nxt)
				i++
				blockComment = false
			}
			continue
		}
		if inStr != 0 {
			cur.WriteByte(c)
			if c == inStr {
				if inStr == '\'' && nxt == '\'' {
					cur.WriteByte(nxt)
					i++
				} else {
					inStr = 0
				}
			}
			continue
		}
		if c == '-' && nxt == '-' {
			lineComment = true
			cur.WriteByte(c)
			cur.WriteByte(nxt)
			i++
			continue
		}
		if c == '/' && nxt == '*' {
			blockComment = true
			cur.WriteByte(c)
			cur.WriteByte(nxt)
			i++
			continue
		}
		if c == '\'' || c == '"' {
			inStr = c
			cur.WriteByte(c)
			continue
		}
		if c == ';' {
			flush()
			continue
		}
		cur.WriteByte(c)
	}
	flush()
	// Orden estable para tests: el orden de aparición se conserva.
	_ = sort.StringSlice{}
	return out
}

// QueryMulti ejecuta N sentencias en orden y devuelve un resultado por
// sentencia (SELECT-like → filas; resto → affected). Máx 20.
func QueryMulti(ctx context.Context, db *sql.DB, sqlText string) ([]QueryResult, error) {
	stmts := SplitStatements(sqlText)
	if len(stmts) == 0 {
		return nil, fmt.Errorf("sql vacío")
	}
	if len(stmts) > 20 {
		return nil, fmt.Errorf("máx 20 sentencias por lote")
	}
	out := make([]QueryResult, 0, len(stmts))
	for _, st := range stmts {
		r, err := Query(ctx, db, st)
		if err != nil {
			return nil, fmt.Errorf("sentencia %d: %w", len(out)+1, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// BuildExplain EXPLAIN (FORMAT JSON) para lectura, EXPLAIN simple si no.
func BuildExplain(sqlText string) (string, bool) {
	t := strings.TrimSpace(sqlText)
	if t == "" {
		return "", false
	}
	if isReadOnly(t) {
		return `EXPLAIN (FORMAT JSON) ` + strings.TrimSuffix(t, ";"), true
	}
	return `EXPLAIN ` + strings.TrimSuffix(t, ";"), false
}

// Explain ejecuta EXPLAIN y devuelve el texto/JSON crudo por filas.
func Explain(ctx context.Context, db *sql.DB, sqlText string) (rows [][]any, cols []string, isJSON bool, err error) {
	q, json := BuildExplain(sqlText)
	if strings.TrimSpace(q) == "" {
		return nil, nil, false, fmt.Errorf("sql vacío")
	}
	rs, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, nil, false, err
	}
	defer rs.Close()
	c, _, r, err := scanRows(rs)
	if err != nil {
		return nil, nil, false, err
	}
	return r, c, json, nil
}
