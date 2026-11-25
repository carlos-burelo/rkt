package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// BuildRenameTable ALTER TABLE ... RENAME TO (mismo esquema).
func BuildRenameTable(schema, old, new string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(old); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(new); err != nil {
		return "", fmt.Errorf("nuevo nombre: %w", err)
	}
	if old == new {
		return "", fmt.Errorf("el nuevo nombre es igual al actual")
	}
	return `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(old) + ` RENAME TO ` + QuoteIdent(new), nil
}

// BuildRenameColumn ALTER TABLE ... RENAME COLUMN.
func BuildRenameColumn(schema, table, old, new string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(old); err != nil {
		return "", fmt.Errorf("columna: %w", err)
	}
	if err := ValidIdent(new); err != nil {
		return "", fmt.Errorf("nuevo nombre: %w", err)
	}
	if old == new {
		return "", fmt.Errorf("el nuevo nombre es igual al actual")
	}
	return `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` RENAME COLUMN ` + QuoteIdent(old) + ` TO ` + QuoteIdent(new), nil
}

// BuildAlterColumnType ALTER COLUMN TYPE con USING (seguro entre tipos).
func BuildAlterColumnType(schema, table, col, newType string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(col); err != nil {
		return "", fmt.Errorf("columna: %w", err)
	}
	if err := ValidColType(newType); err != nil {
		return "", err
	}
	nt := strings.ToLower(strings.TrimSpace(newType))
	return `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` ALTER COLUMN ` + QuoteIdent(col) + ` TYPE ` + nt + ` USING ` + QuoteIdent(col) + `::` + nt, nil
}

// BuildAlterColumnDefault SET/DROP DEFAULT validado.
func BuildAlterColumnDefault(schema, table, col, def string, drop bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(col); err != nil {
		return "", fmt.Errorf("columna: %w", err)
	}
	base := `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) + ` ALTER COLUMN ` + QuoteIdent(col)
	if drop {
		return base + ` DROP DEFAULT`, nil
	}
	if err := ValidColDefault(def); err != nil {
		return "", err
	}
	if strings.TrimSpace(def) == "" {
		return "", fmt.Errorf("default vacío: usa drop para quitarlo")
	}
	return base + ` SET DEFAULT ` + strings.TrimSpace(def), nil
}

// BuildAlterColumnNullable SET/DROP NOT NULL.
func BuildAlterColumnNullable(schema, table, col string, notNull bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(col); err != nil {
		return "", fmt.Errorf("columna: %w", err)
	}
	verb := "DROP NOT NULL"
	if notNull {
		verb = "SET NOT NULL"
	}
	return `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` ALTER COLUMN ` + QuoteIdent(col) + ` ` + verb, nil
}

// BuildTruncate TRUNCATE simple (sin CASCADE: fail-closed).
func BuildTruncate(schema, table string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	return `TRUNCATE TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table), nil
}

// BuildDuplicate devuelve las sentencias para duplicar una tabla:
// CREATE (LIKE ... INCLUDING ALL) + opcional INSERT SELECT.
func BuildDuplicate(schema, table, newTable string, withData bool) ([]string, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return nil, fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(newTable); err != nil {
		return nil, fmt.Errorf("nueva tabla: %w", err)
	}
	if table == newTable {
		return nil, fmt.Errorf("la copia debe tener otro nombre")
	}
	src := QuoteIdent(schema) + `.` + QuoteIdent(table)
	dst := QuoteIdent(schema) + `.` + QuoteIdent(newTable)
	create := `CREATE TABLE ` + dst + ` (LIKE ` + src + ` INCLUDING ALL)`
	if !withData {
		return []string{create}, nil
	}
	return []string{create, `INSERT INTO ` + dst + ` SELECT * FROM ` + src}, nil
}

func execOne(ctx context.Context, db *sql.DB, q string) error {
	_, err := db.ExecContext(ctx, q)
	return err
}

// RenameTableExec renombra una tabla del esquema.
func RenameTableExec(ctx context.Context, db *sql.DB, schema, old, new string) error {
	q, err := BuildRenameTable(schema, old, new)
	if err != nil {
		return err
	}
	return execOne(ctx, db, q)
}

// RenameColumnExec renombra una columna.
func RenameColumnExec(ctx context.Context, db *sql.DB, schema, table, old, new string) error {
	q, err := BuildRenameColumn(schema, table, old, new)
	if err != nil {
		return err
	}
	return execOne(ctx, db, q)
}

// AlterColumnTypeExec cambia el tipo con USING.
func AlterColumnTypeExec(ctx context.Context, db *sql.DB, schema, table, col, newType string) error {
	q, err := BuildAlterColumnType(schema, table, col, newType)
	if err != nil {
		return err
	}
	return execOne(ctx, db, q)
}

// AlterColumnDefaultExec SET/DROP DEFAULT.
func AlterColumnDefaultExec(ctx context.Context, db *sql.DB, schema, table, col, def string, drop bool) error {
	q, err := BuildAlterColumnDefault(schema, table, col, def, drop)
	if err != nil {
		return err
	}
	return execOne(ctx, db, q)
}

// AlterColumnNullableExec SET/DROP NOT NULL.
func AlterColumnNullableExec(ctx context.Context, db *sql.DB, schema, table, col string, notNull bool) error {
	q, err := BuildAlterColumnNullable(schema, table, col, notNull)
	if err != nil {
		return err
	}
	return execOne(ctx, db, q)
}

// TruncateExec vacía una tabla (sin CASCADE).
func TruncateExec(ctx context.Context, db *sql.DB, schema, table string) error {
	q, err := BuildTruncate(schema, table)
	if err != nil {
		return err
	}
	return execOne(ctx, db, q)
}

// DuplicateExec duplica estructura (+datos opcional).
func DuplicateExec(ctx context.Context, db *sql.DB, schema, table, newTable string, withData bool) error {
	stmts, err := BuildDuplicate(schema, table, newTable, withData)
	if err != nil {
		return err
	}
	for _, st := range stmts {
		if err := execOne(ctx, db, st); err != nil {
			return fmt.Errorf("duplicar: %w (%s)", err, st)
		}
	}
	return nil
}
