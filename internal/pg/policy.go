package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// validPolicyName nombres de policy: PG permite casi todo entre comillas;
// aquí solo se vetan vacíos, ; y saltos (una sentencia por ejecución).
func validPolicyName(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 63 {
		return fmt.Errorf("nombre de policy 1..63")
	}
	if strings.ContainsAny(t, ";\n\r\x00") {
		return fmt.Errorf("nombre de policy sin ; ni saltos")
	}
	return nil
}

func validPolicyExpr(label, s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if strings.Contains(s, ";") {
		return fmt.Errorf("%s sin ; (una sentencia por ejecución)", label)
	}
	if len(s) > 4000 {
		return fmt.Errorf("%s máx 4000 caracteres", label)
	}
	return nil
}

// BuildCreatePolicy CREATE POLICY validada (cmd select|insert|update|delete|all).
func BuildCreatePolicy(schema, table, name, cmd string, roles []string, using, check string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := validPolicyName(name); err != nil {
		return "", err
	}
	c := strings.ToLower(strings.TrimSpace(cmd))
	if c == "" {
		c = "all"
	}
	if c != "select" && c != "insert" && c != "update" && c != "delete" && c != "all" {
		return "", fmt.Errorf("cmd %q (select|insert|update|delete|all)", cmd)
	}
	if err := validPolicyExpr("using", using); err != nil {
		return "", err
	}
	if err := validPolicyExpr("check", check); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(`CREATE POLICY ` + QuoteIdent(strings.TrimSpace(name)))
	b.WriteString(` ON ` + QuoteIdent(schema) + `.` + QuoteIdent(table))
	if c != "all" {
		b.WriteString(` FOR ` + strings.ToUpper(c))
	}
	// Roles: lista de idents o PUBLIC. Vacío = aplica a PUBLIC (default PG).
	clean := []string{}
	for _, r := range roles {
		t := strings.TrimSpace(r)
		if t == "" {
			continue
		}
		if strings.EqualFold(t, "public") {
			clean = append(clean, "PUBLIC")
			continue
		}
		if err := ValidIdent(strings.ToLower(t)); err != nil {
			return "", fmt.Errorf("rol %q: %w", r, err)
		}
		clean = append(clean, QuoteIdent(strings.ToLower(t)))
	}
	if len(clean) > 0 {
		b.WriteString(` TO ` + strings.Join(clean, ", "))
	}
	if strings.TrimSpace(using) != "" {
		b.WriteString(` USING (` + strings.TrimSpace(using) + `)`)
	}
	if strings.TrimSpace(check) != "" {
		b.WriteString(` WITH CHECK (` + strings.TrimSpace(check) + `)`)
	}
	return b.String(), nil
}

// BuildDropPolicy DROP POLICY validada.
func BuildDropPolicy(schema, table, name string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := validPolicyName(name); err != nil {
		return "", err
	}
	return `DROP POLICY ` + QuoteIdent(strings.TrimSpace(name)) + ` ON ` + QuoteIdent(schema) + `.` + QuoteIdent(table), nil
}

// CreatePolicyExec crea una policy validada.
func CreatePolicyExec(ctx context.Context, db *sql.DB, schema, table, name, cmd string, roles []string, using, check string) error {
	q, err := BuildCreatePolicy(schema, table, name, cmd, roles, using, check)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// DropPolicyExec borra una policy.
func DropPolicyExec(ctx context.Context, db *sql.DB, schema, table, name string) error {
	q, err := BuildDropPolicy(schema, table, name)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// SetPolicyRoles helper: roles.csv → []roles (vacío = PUBLIC por defecto PG).
func SplitRoles(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
