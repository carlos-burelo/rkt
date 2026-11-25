package pg

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Objetos del esquema (views, funciones, triggers, secuencias): primera
// clase en el Studio. Todo valida identificadores antes de interpolar; los
// valores viajan como parámetros. Testeable sin Postgres.

// ---- views + materialized views ----

// ViewInfo vista o matview del esquema.
type ViewInfo struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"` // view|materialized
	Def     string `json:"def,omitempty"`
	HasData *bool  `json:"has_data,omitempty"`
}

// ListViews views normales del esquema (orden por nombre).
func ListViews(ctx context.Context, db *sql.DB, schema string) ([]ViewInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT viewname FROM pg_views WHERE schemaname=$1 ORDER BY viewname`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ViewInfo{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, ViewInfo{Name: n, Kind: "view"})
	}
	return out, rows.Err()
}

// ListMatviews matviews del esquema con su estado de datos.
func ListMatviews(ctx context.Context, db *sql.DB, schema string) ([]ViewInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT matviewname, ispopulated FROM pg_matviews WHERE schemaname=$1 ORDER BY matviewname`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ViewInfo{}
	for rows.Next() {
		var v ViewInfo
		var pop bool
		if err := rows.Scan(&v.Name, &pop); err != nil {
			return nil, err
		}
		v.Kind = "materialized"
		v.HasData = &pop
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetViewDef definición de una vista o matview (kind detectado solo).
func GetViewDef(ctx context.Context, db *sql.DB, schema, name string) (ViewInfo, error) {
	var out ViewInfo
	if err := ValidIdent(schema); err != nil {
		return out, fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(name); err != nil {
		return out, fmt.Errorf("vista: %w", err)
	}
	var def sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT definition FROM pg_views WHERE schemaname=$1 AND viewname=$2`, schema, name).Scan(&def); err == nil {
		out = ViewInfo{Name: name, Kind: "view"}
		if def.Valid {
			out.Def = def.String
		}
		return out, nil
	}
	var pop bool
	if err := db.QueryRowContext(ctx,
		`SELECT definition, ispopulated FROM pg_matviews WHERE schemaname=$1 AND matviewname=$2`, schema, name).Scan(&def, &pop); err == nil {
		out = ViewInfo{Name: name, Kind: "materialized", HasData: &pop}
		if def.Valid {
			out.Def = def.String
		}
		return out, nil
	}
	return out, fmt.Errorf("vista %q inexistente", name)
}

// BuildCreateView CREATE [OR REPLACE] [MATERIALIZED] VIEW validada.
// El SELECT debe ser lectura (mismo criterio que la consola) y de una
// sola sentencia.
func BuildCreateView(schema, name, selectSQL string, materialized, orReplace, withData bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(name); err != nil {
		return "", fmt.Errorf("vista: %w", err)
	}
	sel := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(selectSQL), ";"))
	if sel == "" {
		return "", fmt.Errorf("select vacío")
	}
	if strings.Contains(sel, ";") {
		return "", fmt.Errorf("select sin ; (una sola sentencia)")
	}
	if !isReadOnly(sel) {
		return "", fmt.Errorf("la vista exige SELECT/WITH (lectura)")
	}
	var b strings.Builder
	b.WriteString("CREATE ")
	if orReplace {
		if materialized {
			return "", fmt.Errorf("OR REPLACE no vale para matviews")
		}
		b.WriteString("OR REPLACE ")
	}
	if materialized {
		b.WriteString("MATERIALIZED ")
	}
	b.WriteString("VIEW " + QuoteIdent(schema) + "." + QuoteIdent(name) + " AS " + sel)
	if materialized {
		if withData {
			b.WriteString(" WITH DATA")
		} else {
			b.WriteString(" WITH NO DATA")
		}
	}
	return b.String(), nil
}

// BuildDropView DROP VIEW o MATERIALIZED VIEW.
func BuildDropView(schema, name string, materialized bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(name); err != nil {
		return "", fmt.Errorf("vista: %w", err)
	}
	kind := "VIEW"
	if materialized {
		kind = "MATERIALIZED VIEW"
	}
	return `DROP ` + kind + ` ` + QuoteIdent(schema) + `.` + QuoteIdent(name), nil
}

// BuildRefreshMatview REFRESH MATERIALIZED VIEW con o sin datos.
func BuildRefreshMatview(schema, name string, withData bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(name); err != nil {
		return "", fmt.Errorf("vista: %w", err)
	}
	q := `REFRESH MATERIALIZED VIEW ` + QuoteIdent(schema) + `.` + QuoteIdent(name)
	if withData {
		q += ` WITH DATA`
	} else {
		q += ` WITH NO DATA`
	}
	return q, nil
}

// CreateViewExec crea vista o matview validada.
func CreateViewExec(ctx context.Context, db *sql.DB, schema, name, selectSQL string, materialized, orReplace, withData bool) error {
	q, err := BuildCreateView(schema, name, selectSQL, materialized, orReplace, withData)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// DropViewExec borra vista o matview.
func DropViewExec(ctx context.Context, db *sql.DB, schema, name string, materialized bool) error {
	q, err := BuildDropView(schema, name, materialized)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// RefreshMatviewExec refresca una matview.
func RefreshMatviewExec(ctx context.Context, db *sql.DB, schema, name string, withData bool) error {
	q, err := BuildRefreshMatview(schema, name, withData)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// ---- funciones y procedimientos ----

// FuncInfo función o procedimiento del esquema.
type FuncInfo struct {
	Name     string `json:"name"`
	Identity string `json:"identity"` // nombre(args) para DROP/CALL
	Args     string `json:"args"`
	Return   string `json:"return"`
	Lang     string `json:"lang"`
	Kind     string `json:"kind"` // function|procedure
	Def      string `json:"def,omitempty"`
}

// listRoutines lista pg_proc por prokind (f = función, p = procedimiento).
func listRoutines(ctx context.Context, db *sql.DB, schema string, kind byte) ([]FuncInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	label := "function"
	if kind == 'p' {
		label = "procedure"
	}
	rows, err := db.QueryContext(ctx,
		`SELECT p.proname, pg_get_function_identity_arguments(p.oid), pg_get_function_result(p.oid), l.lanname
		 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace JOIN pg_language l ON l.oid=p.prolang
		 WHERE n.nspname=$1 AND p.prokind=$2 ORDER BY p.proname`,
		schema, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FuncInfo{}
	for rows.Next() {
		var f FuncInfo
		if err := rows.Scan(&f.Name, &f.Args, &f.Return, &f.Lang); err != nil {
			return nil, err
		}
		f.Kind = label
		f.Identity = f.Name + "(" + f.Args + ")"
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListFunctions funciones del esquema.
func ListFunctions(ctx context.Context, db *sql.DB, schema string) ([]FuncInfo, error) {
	return listRoutines(ctx, db, schema, 'f')
}

// ListProcedures procedimientos del esquema.
func ListProcedures(ctx context.Context, db *sql.DB, schema string) ([]FuncInfo, error) {
	return listRoutines(ctx, db, schema, 'p')
}

var funcIdentRe = regexp.MustCompile(`^[a-z_][a-z0-9_$]*\(.*\)$`)

// splitIdent parte "nombre(args)" en nombre y "(args)".
func splitIdent(identity string) (name, args string, err error) {
	i := strings.Index(identity, "(")
	if i < 0 || !strings.HasSuffix(identity, ")") {
		return "", "", fmt.Errorf("identidad %q: usa nombre(args)", identity)
	}
	name, args = identity[:i], identity[i:]
	if err := ValidIdent(strings.ToLower(name)); err != nil {
		return "", "", fmt.Errorf("función: %w", err)
	}
	if len(identity) > 500 {
		return "", "", fmt.Errorf("identidad demasiado larga")
	}
	if strings.Contains(identity, ";") {
		return "", "", fmt.Errorf("identidad sin ;")
	}
	return strings.ToLower(name), args, nil
}

// qualifiedRoutine "esquema"."nombre"(args) para regprocedure y DROP/CALL.
func qualifiedRoutine(schema, identity string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	name, args, err := splitIdent(identity)
	if err != nil {
		return "", err
	}
	return QuoteIdent(schema) + "." + QuoteIdent(name) + args, nil
}

// GetRoutineDef definición vía pg_get_functiondef (vale para f y p).
func GetRoutineDef(ctx context.Context, db *sql.DB, schema, identity string) (FuncInfo, error) {
	var out FuncInfo
	qual, err := qualifiedRoutine(schema, identity)
	if err != nil {
		return out, err
	}
	name, args, _ := splitIdent(identity)
	var def sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT pg_get_functiondef($1::regprocedure)`, qual).Scan(&def); err != nil {
		return out, fmt.Errorf("rutina %q inexistente: %w", identity, err)
	}
	out = FuncInfo{Name: name, Identity: identity, Args: strings.Trim(args, "()"), Kind: "routine"}
	if def.Valid {
		out.Def = def.String
	}
	return out, nil
}

// BuildDropRoutine DROP FUNCTION o PROCEDURE.
func BuildDropRoutine(schema, identity string, isProcedure bool) (string, error) {
	qual, err := qualifiedRoutine(schema, identity)
	if err != nil {
		return "", err
	}
	kind := "FUNCTION"
	if isProcedure {
		kind = "PROCEDURE"
	}
	return `DROP ` + kind + ` ` + qual, nil
}

// DropRoutineExec borra función o procedimiento.
func DropRoutineExec(ctx context.Context, db *sql.DB, schema, identity string, isProcedure bool) error {
	q, err := BuildDropRoutine(schema, identity, isProcedure)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// BuildCallProcedure CALL "esquema"."nombre"(args literales).
func BuildCallProcedure(schema, call string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	t := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(call), ";"))
	if strings.Contains(t, ";") {
		return "", fmt.Errorf("call sin ; (una sola sentencia)")
	}
	name, args, err := splitIdent(t)
	if err != nil {
		return "", err
	}
	if len(args) > 2000 {
		return "", fmt.Errorf("argumentos máx 2000 caracteres")
	}
	return `CALL ` + QuoteIdent(schema) + `.` + QuoteIdent(name) + args, nil
}

// CallProcedureExec ejecuta CALL (vía Query: devuelve affected).
func CallProcedureExec(ctx context.Context, db *sql.DB, schema, call string) (QueryResult, error) {
	q, err := BuildCallProcedure(schema, call)
	if err != nil {
		return QueryResult{}, err
	}
	return Query(ctx, db, q)
}

// ---- triggers ----

// TriggerInfo trigger de una tabla (una fila por trigger, eventos juntos).
type TriggerInfo struct {
	Name        string `json:"name"`
	Table       string `json:"table"`
	Timing      string `json:"timing"`
	Events      string `json:"events"`
	Orientation string `json:"orientation"`
	Function    string `json:"function"`
	Enabled     string `json:"enabled,omitempty"`
}

// ListTriggers triggers del esquema (todas o una tabla).
func ListTriggers(ctx context.Context, db *sql.DB, schema, table string) ([]TriggerInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	if table != "" {
		if err := ValidIdent(table); err != nil {
			return nil, fmt.Errorf("tabla: %w", err)
		}
	}
	q := `SELECT t.trigger_name, t.event_object_table, t.action_timing,
		string_agg(t.event_manipulation, ',' ORDER BY t.event_manipulation),
		t.action_statement, t.action_orientation
		FROM information_schema.triggers t WHERE t.trigger_schema=$1`
	args := []any{schema}
	if table != "" {
		q += ` AND t.event_object_table=$2`
		args = append(args, table)
	}
	q += ` GROUP BY 1,2,3,5,6 ORDER BY 2,1`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TriggerInfo{}
	for rows.Next() {
		var tr TriggerInfo
		if err := rows.Scan(&tr.Name, &tr.Table, &tr.Timing, &tr.Events, &tr.Function, &tr.Orientation); err != nil {
			return nil, err
		}
		out = append(out, tr)
	}
	if out == nil {
		out = []TriggerInfo{}
	}
	return out, rows.Err()
}

var triggerTimings = map[string]bool{"before": true, "after": true, "instead of": true}
var triggerEvents = map[string]bool{"insert": true, "update": true, "delete": true, "truncate": true}

// BuildCreateTrigger CREATE TRIGGER básico (EXECUTE FUNCTION).
func BuildCreateTrigger(schema, table, name, timing string, events []string, orientation, function, condition string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(strings.ToLower(strings.TrimSpace(name))); err != nil {
		return "", fmt.Errorf("trigger: %w", err)
	}
	tm := strings.ToLower(strings.TrimSpace(timing))
	if !triggerTimings[tm] {
		return "", fmt.Errorf("timing %q (before|after|instead of)", timing)
	}
	if len(events) == 0 || len(events) > 4 {
		return "", fmt.Errorf("eventos 1..4 (insert|update|delete|truncate)")
	}
	seen := map[string]bool{}
	evs := make([]string, 0, len(events))
	for _, e := range events {
		le := strings.ToLower(strings.TrimSpace(e))
		if !triggerEvents[le] {
			return "", fmt.Errorf("evento %q (insert|update|delete|truncate)", e)
		}
		if seen[le] {
			return "", fmt.Errorf("evento %q duplicado", e)
		}
		seen[le] = true
		evs = append(evs, strings.ToUpper(le))
	}
	or := strings.ToLower(strings.TrimSpace(orientation))
	if or == "" {
		or = "row"
	}
	if or != "row" && or != "statement" {
		return "", fmt.Errorf("orientación %q (row|statement)", orientation)
	}
	fn := strings.TrimSpace(function)
	if fn == "" || len(fn) > 300 || strings.Contains(fn, ";") {
		return "", fmt.Errorf("función inválida (nombre(args), sin ;)")
	}
	if !regexp.MustCompile(`^[a-z_][a-z0-9_$\.]+\(.*\)$`).MatchString(strings.ToLower(fn)) {
		return "", fmt.Errorf("función %q: usa nombre(args)", function)
	}
	cond := strings.TrimSpace(condition)
	if cond != "" {
		if len(cond) > 1000 || strings.Contains(cond, ";") {
			return "", fmt.Errorf("condición WHEN sin ; (máx 1000)")
		}
	}
	var b strings.Builder
	b.WriteString(`CREATE TRIGGER ` + QuoteIdent(strings.ToLower(strings.TrimSpace(name))))
	b.WriteString(` ` + strings.ToUpper(tm) + ` ` + strings.Join(evs, " OR "))
	b.WriteString(` ON ` + QuoteIdent(schema) + `.` + QuoteIdent(table))
	b.WriteString(` FOR EACH ` + strings.ToUpper(or))
	if cond != "" {
		b.WriteString(` WHEN (` + cond + `)`)
	}
	b.WriteString(` EXECUTE FUNCTION ` + fn)
	return b.String(), nil
}

// BuildDropTrigger DROP TRIGGER de una tabla.
func BuildDropTrigger(schema, table, name string) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(strings.ToLower(strings.TrimSpace(name))); err != nil {
		return "", fmt.Errorf("trigger: %w", err)
	}
	return `DROP TRIGGER ` + QuoteIdent(strings.ToLower(strings.TrimSpace(name))) +
		` ON ` + QuoteIdent(schema) + `.` + QuoteIdent(table), nil
}

// BuildToggleTrigger ALTER TABLE ... ENABLE/DISABLE TRIGGER.
func BuildToggleTrigger(schema, table, name string, enable bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(table); err != nil {
		return "", fmt.Errorf("tabla: %w", err)
	}
	if err := ValidIdent(strings.ToLower(strings.TrimSpace(name))); err != nil {
		return "", fmt.Errorf("trigger: %w", err)
	}
	verb := "DISABLE"
	if enable {
		verb = "ENABLE"
	}
	return `ALTER TABLE ` + QuoteIdent(schema) + `.` + QuoteIdent(table) +
		` ` + verb + ` TRIGGER ` + QuoteIdent(strings.ToLower(strings.TrimSpace(name))), nil
}

// CreateTriggerExec crea un trigger validado.
func CreateTriggerExec(ctx context.Context, db *sql.DB, schema, table, name, timing string, events []string, orientation, function, condition string) error {
	q, err := BuildCreateTrigger(schema, table, name, timing, events, orientation, function, condition)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// DropTriggerExec borra un trigger.
func DropTriggerExec(ctx context.Context, db *sql.DB, schema, table, name string) error {
	q, err := BuildDropTrigger(schema, table, name)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// ToggleTriggerExec activa o desactiva un trigger.
func ToggleTriggerExec(ctx context.Context, db *sql.DB, schema, table, name string, enable bool) error {
	q, err := BuildToggleTrigger(schema, table, name, enable)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// ---- secuencias ----

// SeqInfo secuencia del esquema.
type SeqInfo struct {
	Name      string `json:"name"`
	DataType  string `json:"data_type"`
	Start     int64  `json:"start"`
	Min       int64  `json:"min"`
	Max       int64  `json:"max"`
	Increment int64  `json:"increment"`
	LastValue *int64 `json:"last_value,omitempty"`
}

// ListSequences secuencias del esquema (orden por nombre).
func ListSequences(ctx context.Context, db *sql.DB, schema string) ([]SeqInfo, error) {
	if err := ValidIdent(schema); err != nil {
		return nil, fmt.Errorf("esquema: %w", err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT sequencename, data_type, start_value, min_value, max_value, increment_by, last_value
		 FROM pg_sequences WHERE schemaname=$1 ORDER BY sequencename`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SeqInfo{}
	for rows.Next() {
		var s SeqInfo
		var last sql.NullInt64
		if err := rows.Scan(&s.Name, &s.DataType, &s.Start, &s.Min, &s.Max, &s.Increment, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			v := last.Int64
			s.LastValue = &v
		}
		out = append(out, s)
	}
	if out == nil {
		out = []SeqInfo{}
	}
	return out, rows.Err()
}

// BuildRestartSequence ALTER SEQUENCE ... RESTART (con valor opcional).
func BuildRestartSequence(schema, name string, with *int64) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(name); err != nil {
		return "", fmt.Errorf("secuencia: %w", err)
	}
	q := `ALTER SEQUENCE ` + QuoteIdent(schema) + `.` + QuoteIdent(name) + ` RESTART`
	if with != nil {
		q += fmt.Sprintf(` WITH %d`, *with)
	}
	return q, nil
}

// BuildSetval SELECT setval('esquema.sec', valor, is_called).
func BuildSetval(schema, name string, value int64, isCalled bool) (string, error) {
	if err := ValidIdent(schema); err != nil {
		return "", fmt.Errorf("esquema: %w", err)
	}
	if err := ValidIdent(name); err != nil {
		return "", fmt.Errorf("secuencia: %w", err)
	}
	called := "true"
	if !isCalled {
		called = "false"
	}
	// El nombre viaja como literal con '' duplicadas (validado arriba).
	lit := strings.ReplaceAll(schema+`.`+name, `'`, `''`)
	return fmt.Sprintf(`SELECT setval('%s', %d, %s)`, lit, value, called), nil
}

// RestartSequenceExec reinicia una secuencia.
func RestartSequenceExec(ctx context.Context, db *sql.DB, schema, name string, with *int64) error {
	q, err := BuildRestartSequence(schema, name, with)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, q)
	return err
}

// SetvalExec fija el valor de una secuencia (devuelve el nuevo valor).
func SetvalExec(ctx context.Context, db *sql.DB, schema, name string, value int64, isCalled bool) (int64, error) {
	q, err := BuildSetval(schema, name, value, isCalled)
	if err != nil {
		return 0, err
	}
	var out int64
	if err := db.QueryRowContext(ctx, q).Scan(&out); err != nil {
		return 0, err
	}
	return out, nil
}
