// Tools extendidas del MCP: control total (rutas, DNS, flota, settings,
// usuarios, observabilidad, backup y poda). Mismo backend que CLI/API.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rkt/internal/alerts"
	"rkt/internal/appview"
	"rkt/internal/cfgfile"
	"rkt/internal/config"
	"rkt/internal/deploy"
	"rkt/internal/health"
	"rkt/internal/hosts"
	"rkt/internal/logx"
	"rkt/internal/pg"
	"rkt/internal/settings"
	"rkt/internal/store"
	"rkt/internal/weblog"
)

type nameArg struct {
	App string `json:"app" jsonschema:"nombre de la app"`
}
type routesAddArg struct {
	Hosts  []string `json:"hosts,omitempty" jsonschema:"hosts a enrutar (vacío = cualquier host/IP)"`
	Target string   `json:"target" jsonschema:"backend http(s)://127.0.0.1:puerto"`
	Path   string   `json:"path,omitempty" jsonschema:"prefijo de ruta (ej /app, se recorta al proxear)"`
}
type routesUpdateArg struct {
	Host   string `json:"host" jsonschema:"host existente en la ruta"`
	Target string `json:"target" jsonschema:"nuevo backend"`
}
type hostArg struct {
	Host string `json:"host,omitempty" jsonschema:"host a quitar"`
	Path string `json:"path,omitempty" jsonschema:"prefijo a quitar (rutas sin host)"`
}
type verifyArg struct {
	Host string `json:"host" jsonschema:"host a verificar"`
}
type envDelArg struct {
	App string `json:"app" jsonschema:"nombre de la app"`
	Key string `json:"key" jsonschema:"CLAVE"`
}
type envGlobalArg struct {
	Key   string `json:"key" jsonschema:"CLAVE a definir"`
	Value string `json:"value" jsonschema:"valor (admite ${VAR})"`
}
type envGlobalListArg struct {
	IncludeSecrets bool `json:"include_secrets,omitempty" jsonschema:"incluir valores (default false)"`
}
type deployWaitArg struct {
	ID       int64  `json:"id,omitempty" jsonschema:"id del deployment (o usa app)"`
	App      string `json:"app,omitempty" jsonschema:"esperar el último deployment de la app"`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"tope segundos (default 120, máx 600)"`
}
type logSearchArg struct {
	App     string `json:"app,omitempty" jsonschema:"app (vacío = operativo)"`
	Pattern string `json:"pattern" jsonschema:"regex a buscar"`
	N       int    `json:"n,omitempty" jsonschema:"máximo de líneas (default 50)"`
}
type settingsSetArg struct {
	Key   string `json:"key" jsonschema:"clave de configuración"`
	Value string `json:"value" jsonschema:"valor"`
}
type alertAddArg struct {
	App       string  `json:"app,omitempty" jsonschema:"app o *"`
	Kind      string  `json:"kind" jsonschema:"failed|error_rate"`
	Threshold float64 `json:"threshold,omitempty" jsonschema:"umbral (0.1 = 10% en error_rate)"`
	WindowMin int     `json:"window_min,omitempty" jsonschema:"ventana minutos (default 60)"`
}
type alertDelArg struct {
	ID int64 `json:"id" jsonschema:"id de la regla"`
}
type alertUpdateArg struct {
	ID        int64    `json:"id" jsonschema:"id de la regla"`
	App       string   `json:"app,omitempty" jsonschema:"app o *"`
	Kind      string   `json:"kind,omitempty" jsonschema:"failed|error_rate|latency_p95|memory_mb|down_min|disk_pct|cert_days|deploy_failed"`
	Threshold *float64 `json:"threshold,omitempty" jsonschema:"umbral"`
	WindowMin *int     `json:"window_min,omitempty" jsonschema:"ventana minutos"`
	Enabled   *bool    `json:"enabled,omitempty" jsonschema:"activar/desactivar"`
	ChannelID *int64   `json:"channel_id,omitempty" jsonschema:"canal (0 = sin canal)"`
}
type userAddArg struct {
	Name     string `json:"name" jsonschema:"usuario"`
	Password string `json:"password" jsonschema:"contraseña (mín 8)"`
	Role     string `json:"role,omitempty" jsonschema:"viewer|operator|admin (default viewer)"`
}
type userArg struct {
	Name string `json:"name" jsonschema:"usuario"`
}
type userPassArg struct {
	Name     string `json:"name" jsonschema:"usuario"`
	Password string `json:"password" jsonschema:"contraseña nueva (mín 8)"`
}
type exportArg struct {
	IncludeSecrets bool `json:"include_secrets,omitempty" jsonschema:"incluir valores de Env (default false)"`
}
type importArg struct {
	JSON    string `json:"json" jsonschema:"config JSON completa (export de config_export)"`
	Confirm string `json:"confirm" jsonschema:"escribe replace para confirmar"`
}
type pruneArg struct {
	DeploymentsDays int    `json:"deployments_days,omitempty" jsonschema:"borrar deployments terminados con más días (default 30)"`
	EventsDays      int    `json:"events_days,omitempty" jsonschema:"borrar eventos con más días (default 30)"`
	Confirm         string `json:"confirm" jsonschema:"escribe prune para confirmar"`
}
type logsClearArg struct {
	Confirm string `json:"confirm" jsonschema:"escribe clear para confirmar"`
}
type pgProvisionArg struct {
	App  string `json:"app" jsonschema:"app a la que se le crea esquema+rol"`
	Mode string `json:"mode,omitempty" jsonschema:"dedicated (default) o shared"`
}
type pgAppArg struct {
	App string `json:"app" jsonschema:"app con esquema aprovisionado"`
}
type pgDropArg struct {
	App     string `json:"app" jsonschema:"app cuyo esquema+rol se borra"`
	Confirm string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgTablesArg struct {
	App string `json:"app" jsonschema:"app con esquema aprovisionado"`
}

type pgRowsArg struct {
	App     string   `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string   `json:"table" jsonschema:"tabla del esquema"`
	N       int      `json:"n,omitempty" jsonschema:"filas por página (default 100, máx 500)"`
	Page    int      `json:"page,omitempty" jsonschema:"página (default 1)"`
	Order   string   `json:"order,omitempty" jsonschema:"columna de orden"`
	Dir     string   `json:"dir,omitempty" jsonschema:"asc o desc"`
	Filters []string `json:"filters,omitempty" jsonschema:"filtros col:op:val (eq,neq,gt,gte,lt,lte,like,ilike,in,isnull,isnotnull)"`
	Search  string   `json:"search,omitempty" jsonschema:"búsqueda global en columnas de texto"`
}

type pgSchemaArg struct {
	App   string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table string `json:"table,omitempty" jsonschema:"tabla (vacío = todas)"`
}

type pgInsertArg struct {
	App     string         `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string         `json:"table" jsonschema:"tabla del esquema"`
	Data    map[string]any `json:"data" jsonschema:"fila a insertar (objeto col->valor)"`
	Confirm string         `json:"confirm" jsonschema:"escribe insert para confirmar"`
}

type pgUpdateArg struct {
	App     string         `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string         `json:"table" jsonschema:"tabla del esquema"`
	PK      map[string]any `json:"pk" jsonschema:"clave primaria exacta (col->valor)"`
	Patch   map[string]any `json:"patch" jsonschema:"columnas a cambiar (col->valor)"`
	Confirm string         `json:"confirm" jsonschema:"escribe update para confirmar"`
}

type pgDeleteArg struct {
	App     string         `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string         `json:"table" jsonschema:"tabla del esquema"`
	PK      map[string]any `json:"pk,omitempty" jsonschema:"clave primaria exacta (o filters)"`
	Filters []string       `json:"filters,omitempty" jsonschema:"filtros col:op:val (o pk)"`
	Confirm string         `json:"confirm" jsonschema:"escribe delete para confirmar"`
}

type pgQueryArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	SQL     string `json:"sql" jsonschema:"una sola sentencia SQL"`
	Confirm string `json:"confirm" jsonschema:"escribe query para confirmar (toda ejecución es mutación potencial)"`
}

type pgRestoreArg struct {
	App     string `json:"app" jsonschema:"app destino (debe tener esquema)"`
	File    string `json:"file" jsonschema:"ruta del dump custom (-Fc)"`
	Confirm string `json:"confirm" jsonschema:"escribe restore para confirmar (sobrescribe datos)"`
}

type pgIndexArg struct {
	App   string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table string `json:"table" jsonschema:"tabla del esquema"`
}

type pgCreateIndexArg struct {
	App     string   `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string   `json:"table" jsonschema:"tabla del esquema"`
	Name    string   `json:"name" jsonschema:"nombre del índice"`
	Columns []string `json:"columns" jsonschema:"columnas del índice"`
	Unique  bool     `json:"unique,omitempty" jsonschema:"índice único (default false)"`
	Method  string   `json:"method,omitempty" jsonschema:"btree|hash|gin|gist (default btree)"`
	Confirm string   `json:"confirm" jsonschema:"escribe create para confirmar"`
}

type pgDropIndexArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Index   string `json:"index" jsonschema:"nombre del índice"`
	Confirm string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgConstraintsArg struct {
	App   string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table string `json:"table" jsonschema:"tabla del esquema"`
}

type pgPolicyCreateArg struct {
	App     string   `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string   `json:"table" jsonschema:"tabla del esquema"`
	Name    string   `json:"name" jsonschema:"nombre de la policy"`
	Cmd     string   `json:"cmd,omitempty" jsonschema:"ALL|SELECT|INSERT|UPDATE|DELETE (default ALL)"`
	Roles   []string `json:"roles,omitempty" jsonschema:"roles (vacío = PUBLIC)"`
	Using   string   `json:"using,omitempty" jsonschema:"expresión USING"`
	Check   string   `json:"check,omitempty" jsonschema:"expresión WITH CHECK"`
	Confirm string   `json:"confirm" jsonschema:"escribe create para confirmar"`
}

type pgPolicyDropArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string `json:"table" jsonschema:"tabla del esquema"`
	Name    string `json:"name" jsonschema:"nombre de la policy"`
	Confirm string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgRenameArg struct {
	App      string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table    string `json:"table" jsonschema:"tabla actual"`
	NewTable string `json:"new_table" jsonschema:"nuevo nombre de la tabla"`
	Confirm  string `json:"confirm" jsonschema:"escribe alter para confirmar"`
}

type pgRenameColArg struct {
	App       string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table     string `json:"table" jsonschema:"tabla del esquema"`
	Column    string `json:"column" jsonschema:"columna actual"`
	NewColumn string `json:"new_column" jsonschema:"nuevo nombre de la columna"`
	Confirm   string `json:"confirm" jsonschema:"escribe alter para confirmar"`
}

type pgAlterColArg struct {
	App         string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table       string `json:"table" jsonschema:"tabla del esquema"`
	Column      string `json:"column" jsonschema:"columna a alterar"`
	Type        string `json:"type,omitempty" jsonschema:"nuevo tipo (ej text, bigint)"`
	Default     string `json:"default,omitempty" jsonschema:"nuevo DEFAULT (expresión SQL)"`
	DropDefault bool   `json:"drop_default,omitempty" jsonschema:"quita el DEFAULT (default false)"`
	NotNull     *bool  `json:"not_null,omitempty" jsonschema:"NOT NULL (verdadero lo pone, falso lo quita)"`
	Confirm     string `json:"confirm" jsonschema:"escribe alter para confirmar"`
}

type pgTruncateArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string `json:"table" jsonschema:"tabla a vaciar"`
	Confirm string `json:"confirm" jsonschema:"escribe truncate para confirmar"`
}

type pgDuplicateArg struct {
	App      string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table    string `json:"table" jsonschema:"tabla origen"`
	NewTable string `json:"new_table" jsonschema:"tabla destino (no debe existir)"`
	WithData bool   `json:"with_data,omitempty" jsonschema:"copiar datos además del esquema (default false)"`
	Confirm  string `json:"confirm" jsonschema:"escribe duplicate para confirmar"`
}

type pgBackupsArg struct {
	App string `json:"app" jsonschema:"app con esquema aprovisionado"`
}

type pgBackupDelArg struct {
	App     string `json:"app" jsonschema:"app dueña del backup"`
	Name    string `json:"name" jsonschema:"nombre del archivo (pg-<app>-*.dump)"`
	Confirm string `json:"confirm" jsonschema:"escribe delete para confirmar"`
}

type pgImportArg struct {
	App     string   `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string   `json:"table" jsonschema:"tabla destino"`
	Columns []string `json:"columns" jsonschema:"columnas (1..100)"`
	Rows    [][]any  `json:"rows" jsonschema:"filas (1..1000, mismo ancho que columns)"`
	Confirm string   `json:"confirm" jsonschema:"escribe import para confirmar"`
}

type pgBatchArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	SQL     string `json:"sql" jsonschema:"lote SQL (máx 20 sentencias)"`
	Confirm string `json:"confirm" jsonschema:"escribe batch para confirmar"`
}

type pgExplainArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	SQL     string `json:"sql" jsonschema:"sentencia a explicar"`
	Confirm string `json:"confirm" jsonschema:"escribe explain para confirmar"`
}

func jsonText(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// load abre config+DB y la cierra al terminar el handler.
func (s *Server) load() (*config.Config, *store.DB, error) {
	return cfgfile.LoadMerged(s.cfgPath)
}

func (s *Server) doAppGet(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	return jsonText(appview.App(cfg, *a))
}

// pgReg adapta store.DB al Registry mínimo de pg.
type pgReg struct{ db *store.DB }

func (r pgReg) PgGet(app string) (pg.Record, bool, error) {
	p, ok, err := r.db.PgGet(app)
	if err != nil || !ok {
		return pg.Record{}, ok, err
	}
	return pg.Record{App: p.App, Schema: p.Schema, Role: p.Role, Mode: p.Mode, DB: p.DB}, true, nil
}

func (r pgReg) PgList() ([]pg.Record, error) {
	all, err := r.db.PgList()
	if err != nil {
		return nil, err
	}
	out := make([]pg.Record, 0, len(all))
	for _, p := range all {
		out = append(out, pg.Record{App: p.App, Schema: p.Schema, Role: p.Role, Mode: p.Mode, DB: p.DB})
	}
	return out, nil
}

func (r pgReg) PgPut(app, schema, role, mode, db string) error {
	return r.db.PgProvision(app, schema, role, mode, db)
}

func (r pgReg) PgRemove(app string) error {
	return r.db.PgRemove(app)
}

func (s *Server) doPgStatus() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return jsonText(map[string]any{"enabled": false})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, cfg.User, cfg.Pass, "postgres"))
	if err != nil {
		return "", err
	}
	defer sdb.Close()
	v, err := pg.Version(ctx, sdb)
	if err != nil {
		return "", err
	}
	if i := strings.Index(v, " "); i > 0 {
		if j := strings.Index(v[i+1:], " "); j > 0 {
			v = v[:i+1+j]
		}
	}
	return jsonText(map[string]any{"enabled": true, "host": cfg.Host, "port": cfg.Port, "database": cfg.DB, "version": v})
}

func (s *Server) doPgList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	recs, err := pgReg{db}.PgList()
	if err != nil {
		return "", err
	}
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return jsonText(map[string]any{"enabled": false, "apps": []any{}})
	}
	type row struct {
		App    string `json:"app"`
		Schema string `json:"schema"`
		Role   string `json:"role"`
		Mode   string `json:"mode"`
		URL    string `json:"url"`
	}
	rows := []row{}
	for _, r := range recs {
		rows = append(rows, row{r.App, r.Schema, r.Role, r.Mode, pg.ConnectionURL(cfg.Host, cfg.Port, r.DB, r.Role)})
	}
	return jsonText(map[string]any{"enabled": true, "database": cfg.DB, "apps": rows})
}

func (s *Server) doPgProvision(a pgProvisionArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	// La base cuelga de una app existente: validar antes de tocar Postgres.
	if cfgfile.FindApp(cfg, a.App) == nil {
		return "", fmt.Errorf("app %q inexistente: créala primero (apps add)", a.App)
	}
	cfg2, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	url, err := pg.ProvisionApp(ctx, pgReg{db}, cfg2, a.App, a.Mode)
	if err != nil {
		return "", err
	}
	if err := cfgfile.SetEnv(db, a.App, "DATABASE_URL", url); err != nil {
		return "", fmt.Errorf("provisionada pero env no: %w", err)
	}
	rec, _, _ := pgReg{db}.PgGet(a.App)
	return jsonText(map[string]any{
		"app": rec.App, "schema": rec.Schema, "role": rec.Role, "mode": rec.Mode, "url": url,
	})
}

func (s *Server) doPgDrop(a pgDropArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := pg.DropApp(ctx, pgReg{db}, cfg, a.App); err != nil {
		return "", err
	}
	return fmt.Sprintf("pg %s borrada (esquema+rol)", a.App), nil
}

func (s *Server) doPgBackup(a pgAppArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out, err := pg.BackupApp(ctx, cfg, pgReg{db}, a.App, filepath.Join(logx.DirFor(s.cfgPath), "..", "data", "backups"))
	if err != nil {
		return "", err
	}
	return out, nil
}

// pgAppDB conecta como rol de la app (trust loopback, sin password).
func (s *Server) pgAppDB(app string) (*pg.Record, *pg.Config, error) {
	_, db, err := s.load()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	rec, ok, err := pgReg{db}.PgGet(strings.TrimSpace(app))
	if err != nil || !ok {
		return nil, nil, fmt.Errorf("pg: %q sin base", app)
	}
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return nil, nil, err
	}
	return &rec, &cfg, nil
}

func (s *Server) doPgTables(a pgTablesArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	tables, err := pg.Tables(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	if tables == nil {
		tables = []string{}
	}
	return jsonText(map[string]any{"app": a.App, "tables": tables})
}

func (s *Server) doPgRows(a pgRowsArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" {
		return "", fmt.Errorf("app y table requeridos")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	n := a.N
	if n <= 0 {
		n = 100
	}
	page := a.Page
	if page < 1 {
		page = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	filters, err := pg.ParseFilters(a.Filters)
	if err != nil {
		return "", err
	}
	cols, rows, total, err := pg.TableRowsEx(ctx, cdb, rec.Schema, a.Table, pg.RowsQuery{
		Limit: n, Offset: (page - 1) * n,
		OrderCol: a.Order, OrderDir: a.Dir, Filters: filters, Search: a.Search,
	})
	if err != nil {
		return "", err
	}
	byName := map[string]string{}
	if cis, cerr := pg.ColumnTypes(ctx, cdb, rec.Schema, a.Table); cerr == nil {
		for _, c := range cis {
			byName[c.Name] = c.Type
		}
	}
	type colJSON struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	typed := make([]colJSON, len(cols))
	for i, c := range cols {
		typed[i] = colJSON{Name: c, Type: byName[c]}
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "columns": cols, "types": typed, "rows": rows, "total": total, "page": page})
}

func (s *Server) doPgQuery(a pgQueryArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.SQL) == "" {
		return "", fmt.Errorf("app y sql requeridos")
	}
	if a.Confirm != "query" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"query"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	res, err := pg.Query(ctx, cdb, a.SQL)
	if err != nil {
		return "", err
	}
	return jsonText(res)
}

func (s *Server) doPgSchema(a pgSchemaArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if strings.TrimSpace(a.Table) != "" {
		s, err := pg.DescribeTable(ctx, cdb, rec.Schema, a.Table)
		if err != nil {
			return "", err
		}
		pols, _ := pg.ListPolicies(ctx, cdb, rec.Schema, a.Table)
		if pols == nil {
			pols = []pg.Policy{}
		}
		return jsonText(map[string]any{"app": a.App, "schema": s, "policies": pols})
	}
	tables, err := pg.Tables(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	out := make([]pg.TableSchema, 0, len(tables))
	for _, t := range tables {
		s, err := pg.DescribeTable(ctx, cdb, rec.Schema, t)
		if err != nil {
			return "", err
		}
		out = append(out, s)
	}
	return jsonText(map[string]any{"app": a.App, "tables": out})
}

func (s *Server) doPgInsert(a pgInsertArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || len(a.Data) == 0 {
		return "", fmt.Errorf("app, table y data requeridos")
	}
	if a.Confirm != "insert" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"insert"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	row, err := pg.InsertRow(ctx, cdb, rec.Schema, a.Table, a.Data)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "row": row})
}

func (s *Server) doPgUpdate(a pgUpdateArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || len(a.PK) == 0 || len(a.Patch) == 0 {
		return "", fmt.Errorf("app, table, pk y patch requeridos")
	}
	if a.Confirm != "update" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"update"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	n, err := pg.UpdateRow(ctx, cdb, rec.Schema, a.Table, a.PK, a.Patch)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "updated": n})
}

func (s *Server) doPgDelete(a pgDeleteArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" {
		return "", fmt.Errorf("app y table requeridos")
	}
	if a.Confirm != "delete" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"delete"}`)
	}
	filters, err := pg.ParseFilters(a.Filters)
	if err != nil {
		return "", err
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	n, err := pg.DeleteRows(ctx, cdb, rec.Schema, a.Table, a.PK, filters)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "deleted": n})
}

func (s *Server) doPgRestore(a pgRestoreArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.File) == "" {
		return "", fmt.Errorf("app y file requeridos")
	}
	if a.Confirm != "restore" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"restore"}`)
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rec, ok, err := pgReg{db}.PgGet(strings.TrimSpace(a.App))
	if err != nil || !ok {
		return "", fmt.Errorf("pg: %q sin base", a.App)
	}
	cfg, err := pg.LoadConfig(db.GetSetting)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := pg.Restore(ctx, cfg.Bins.PgRestore, cfg.Host, cfg.Port, cfg.User, cfg.Pass, cfg.DB, a.File); err != nil {
		return "", err
	}
	_ = rec
	return fmt.Sprintf("pg %s restaurada desde %s", a.App, a.File), nil
}

func (s *Server) doPgIndexes(a pgIndexArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" {
		return "", fmt.Errorf("app y table requeridos")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	idxs, err := pg.ListIndexes(ctx, cdb, rec.Schema, a.Table)
	if err != nil {
		return "", err
	}
	if idxs == nil {
		idxs = []pg.IndexInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "indexes": idxs})
}

func (s *Server) doPgCreateIndex(a pgCreateIndexArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Name) == "" || len(a.Columns) == 0 {
		return "", fmt.Errorf("app, table, name y columns requeridos")
	}
	if a.Confirm != "create" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"create"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.CreateIndexExec(ctx, cdb, rec.Schema, a.Table, a.Name, a.Columns, a.Unique, a.Method); err != nil {
		return "", err
	}
	return fmt.Sprintf("índice %s creado en %s", a.Name, a.Table), nil
}

func (s *Server) doPgDropIndex(a pgDropIndexArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Index) == "" {
		return "", fmt.Errorf("app e index requeridos")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.DropIndexExec(ctx, cdb, rec.Schema, a.Index); err != nil {
		return "", err
	}
	return fmt.Sprintf("índice %s borrado", a.Index), nil
}

func (s *Server) doPgConstraints(a pgConstraintsArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" {
		return "", fmt.Errorf("app y table requeridos")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	cons, err := pg.ListConstraints(ctx, cdb, rec.Schema, a.Table)
	if err != nil {
		return "", err
	}
	if cons == nil {
		cons = []pg.ConstraintInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "constraints": cons})
}

func (s *Server) doPgPolicyCreate(a pgPolicyCreateArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app, table y name requeridos")
	}
	if a.Confirm != "create" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"create"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.CreatePolicyExec(ctx, cdb, rec.Schema, a.Table, a.Name, a.Cmd, a.Roles, a.Using, a.Check); err != nil {
		return "", err
	}
	return fmt.Sprintf("policy %s creada en %s", a.Name, a.Table), nil
}

func (s *Server) doPgPolicyDrop(a pgPolicyDropArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app, table y name requeridos")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.DropPolicyExec(ctx, cdb, rec.Schema, a.Table, a.Name); err != nil {
		return "", err
	}
	return fmt.Sprintf("policy %s borrada de %s", a.Name, a.Table), nil
}

func (s *Server) doPgRenameTable(a pgRenameArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.NewTable) == "" {
		return "", fmt.Errorf("app, table y new_table requeridos")
	}
	if a.Confirm != "alter" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"alter"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.RenameTableExec(ctx, cdb, rec.Schema, a.Table, a.NewTable); err != nil {
		return "", err
	}
	return fmt.Sprintf("tabla %s → %s", a.Table, a.NewTable), nil
}

func (s *Server) doPgRenameColumn(a pgRenameColArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Column) == "" || strings.TrimSpace(a.NewColumn) == "" {
		return "", fmt.Errorf("app, table, column y new_column requeridos")
	}
	if a.Confirm != "alter" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"alter"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.RenameColumnExec(ctx, cdb, rec.Schema, a.Table, a.Column, a.NewColumn); err != nil {
		return "", err
	}
	return fmt.Sprintf("columna %s.%s → %s", a.Table, a.Column, a.NewColumn), nil
}

func (s *Server) doPgAlterColumn(a pgAlterColArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Column) == "" {
		return "", fmt.Errorf("app, table y column requeridos")
	}
	if a.Confirm != "alter" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"alter"}`)
	}
	if strings.TrimSpace(a.Type) == "" && strings.TrimSpace(a.Default) == "" && !a.DropDefault && a.NotNull == nil {
		return "", fmt.Errorf("nada que alterar: indica type, default/drop_default o not_null")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	applied := []string{}
	if strings.TrimSpace(a.Type) != "" {
		if err := pg.AlterColumnTypeExec(ctx, cdb, rec.Schema, a.Table, a.Column, a.Type); err != nil {
			return "", err
		}
		applied = append(applied, "type="+a.Type)
	}
	if a.DropDefault {
		if err := pg.AlterColumnDefaultExec(ctx, cdb, rec.Schema, a.Table, a.Column, "", true); err != nil {
			return "", err
		}
		applied = append(applied, "drop_default")
	} else if strings.TrimSpace(a.Default) != "" {
		if err := pg.AlterColumnDefaultExec(ctx, cdb, rec.Schema, a.Table, a.Column, a.Default, false); err != nil {
			return "", err
		}
		applied = append(applied, "default")
	}
	if a.NotNull != nil {
		if err := pg.AlterColumnNullableExec(ctx, cdb, rec.Schema, a.Table, a.Column, *a.NotNull); err != nil {
			return "", err
		}
		applied = append(applied, "not_null")
	}
	return fmt.Sprintf("columna %s.%s alterada (%s)", a.Table, a.Column, strings.Join(applied, ",")), nil
}

func (s *Server) doPgTruncate(a pgTruncateArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" {
		return "", fmt.Errorf("app y table requeridos")
	}
	if a.Confirm != "truncate" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"truncate"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.TruncateExec(ctx, cdb, rec.Schema, a.Table); err != nil {
		return "", err
	}
	return fmt.Sprintf("tabla %s vaciada", a.Table), nil
}

func (s *Server) doPgDuplicate(a pgDuplicateArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.NewTable) == "" {
		return "", fmt.Errorf("app, table y new_table requeridos")
	}
	if a.Confirm != "duplicate" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"duplicate"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.DuplicateExec(ctx, cdb, rec.Schema, a.Table, a.NewTable, a.WithData); err != nil {
		return "", err
	}
	return fmt.Sprintf("tabla %s duplicada en %s", a.Table, a.NewTable), nil
}

func (s *Server) doPgBackups(a pgBackupsArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	if _, _, err := s.pgAppDB(a.App); err != nil {
		return "", err
	}
	dir := filepath.Join(logx.DirFor(s.cfgPath), "..", "data", "backups")
	list, err := pg.ListBackups(dir, strings.TrimSpace(a.App))
	if err != nil {
		return "", err
	}
	if list == nil {
		list = []pg.BackupInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "backups": list})
}

func (s *Server) doPgBackupDelete(a pgBackupDelArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app y name requeridos")
	}
	if a.Confirm != "delete" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"delete"}`)
	}
	if _, _, err := s.pgAppDB(a.App); err != nil {
		return "", err
	}
	dir := filepath.Join(logx.DirFor(s.cfgPath), "..", "data", "backups")
	if err := pg.DeleteBackup(dir, strings.TrimSpace(a.App), strings.TrimSpace(a.Name)); err != nil {
		return "", err
	}
	return fmt.Sprintf("backup %s borrado", a.Name), nil
}

func (s *Server) doPgImport(a pgImportArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || len(a.Columns) == 0 || len(a.Rows) == 0 {
		return "", fmt.Errorf("app, table, columns y rows requeridos")
	}
	if a.Confirm != "import" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"import"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	if _, _, err := pg.BuildInsertMany(rec.Schema, a.Table, a.Columns, a.Rows); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	n, err := pg.InsertManyExec(ctx, cdb, rec.Schema, a.Table, a.Columns, a.Rows)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"app": a.App, "table": a.Table, "inserted": n})
}

func (s *Server) doPgBatch(a pgBatchArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.SQL) == "" {
		return "", fmt.Errorf("app y sql requeridos")
	}
	if a.Confirm != "batch" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"batch"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	res, err := pg.QueryMulti(ctx, cdb, a.SQL)
	if err != nil {
		return "", err
	}
	return jsonText(res)
}

func (s *Server) doPgExplain(a pgExplainArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.SQL) == "" {
		return "", fmt.Errorf("app y sql requeridos")
	}
	if a.Confirm != "explain" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"explain"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	rows, cols, isJSON, err := pg.Explain(ctx, cdb, a.SQL)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"columns": cols, "rows": rows, "json": isJSON})
}

type pgViewsArg struct {
	App  string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Name string `json:"name,omitempty" jsonschema:"vista o matview (vacío = lista todas)"`
}

type pgViewCreateArg struct {
	App          string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Name         string `json:"name" jsonschema:"nombre de la vista"`
	SQL          string `json:"sql" jsonschema:"SELECT de la vista (solo lectura)"`
	Materialized bool   `json:"materialized,omitempty" jsonschema:"crea matview en vez de vista (default false)"`
	OrReplace    bool   `json:"or_replace,omitempty" jsonschema:"OR REPLACE si ya existe, solo vistas (default false)"`
	WithData     bool   `json:"with_data,omitempty" jsonschema:"matview con datos iniciales (default false)"`
	Confirm      string `json:"confirm" jsonschema:"escribe create para confirmar"`
}

type pgViewDropArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Name    string `json:"name" jsonschema:"vista o matview a borrar"`
	Confirm string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgMatviewRefreshArg struct {
	App      string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Name     string `json:"name" jsonschema:"matview a refrescar"`
	WithData bool   `json:"with_data,omitempty" jsonschema:"refresca con datos (default false = sin datos)"`
	Confirm  string `json:"confirm" jsonschema:"escribe refresh para confirmar"`
}

type pgFunctionsArg struct {
	App  string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Func string `json:"func,omitempty" jsonschema:"identidad nombre(args) para ver su definición (vacío = lista)"`
}

type pgFunctionDropArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Func    string `json:"func" jsonschema:"identidad nombre(args) de la función"`
	Confirm string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgProceduresArg struct {
	App       string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Procedure string `json:"procedure,omitempty" jsonschema:"identidad nombre(args) para ver su definición (vacío = lista)"`
}

type pgProcedureCallArg struct {
	App       string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Procedure string `json:"procedure" jsonschema:"llamada nombre(literales) del procedimiento"`
	Confirm   string `json:"confirm" jsonschema:"escribe call para confirmar"`
}

type pgProcedureDropArg struct {
	App       string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Procedure string `json:"procedure" jsonschema:"identidad nombre(args) del procedimiento"`
	Confirm   string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgTriggersArg struct {
	App   string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table string `json:"table,omitempty" jsonschema:"tabla (vacío = todas)"`
}

// pgEventList acepta "insert,update" o ["insert","update"].
type pgEventList []string

func (l *pgEventList) UnmarshalJSON(b []byte) error {
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*l = splitPgEvents(arr)
		return nil
	}
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*l = splitPgEvents([]string{single})
		return nil
	}
	return fmt.Errorf("events: usa texto con comas o lista de textos")
}

func splitPgEvents(in []string) []string {
	out := []string{}
	for _, e := range in {
		for _, p := range strings.Split(e, ",") {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

type pgTriggerCreateArg struct {
	App         string      `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table       string      `json:"table" jsonschema:"tabla del esquema"`
	Name        string      `json:"name" jsonschema:"nombre del trigger"`
	Timing      string      `json:"timing" jsonschema:"before, after o instead of"`
	Events      pgEventList `json:"events" jsonschema:"eventos insert|update|delete|truncate (texto con comas o lista)"`
	Orientation string      `json:"orientation,omitempty" jsonschema:"row o statement (default row)"`
	Function    string      `json:"function" jsonschema:"función a ejecutar nombre(args)"`
	Condition   string      `json:"condition,omitempty" jsonschema:"condición WHEN sin punto y coma (opcional)"`
	Confirm     string      `json:"confirm" jsonschema:"escribe create para confirmar"`
}

type pgTriggerDropArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string `json:"table" jsonschema:"tabla del esquema"`
	Name    string `json:"name" jsonschema:"nombre del trigger"`
	Confirm string `json:"confirm" jsonschema:"escribe drop para confirmar"`
}

type pgTriggerToggleArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Table   string `json:"table" jsonschema:"tabla del esquema"`
	Name    string `json:"name" jsonschema:"nombre del trigger"`
	Enable  bool   `json:"enable,omitempty" jsonschema:"verdadero lo activa, falso lo desactiva"`
	Confirm string `json:"confirm" jsonschema:"escribe toggle para confirmar"`
}

type pgSequencesArg struct {
	App string `json:"app" jsonschema:"app con esquema aprovisionado"`
}

type pgSequenceRestartArg struct {
	App     string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Name    string `json:"name" jsonschema:"nombre de la secuencia"`
	With    int64  `json:"with,omitempty" jsonschema:"valor de reinicio (0 = sin valor)"`
	Confirm string `json:"confirm" jsonschema:"escribe restart para confirmar"`
}

type pgSequenceSetvalArg struct {
	App      string `json:"app" jsonschema:"app con esquema aprovisionado"`
	Name     string `json:"name" jsonschema:"nombre de la secuencia"`
	Value    int64  `json:"value" jsonschema:"nuevo valor de la secuencia"`
	IsCalled bool   `json:"is_called,omitempty" jsonschema:"verdadero si el valor ya se usó (default false)"`
	Confirm  string `json:"confirm" jsonschema:"escribe setval para confirmar"`
}

func (s *Server) doPgViews(a pgViewsArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if strings.TrimSpace(a.Name) != "" {
		v, err := pg.GetViewDef(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name))
		if err != nil {
			return "", err
		}
		return jsonText(map[string]any{"app": a.App, "view": v})
	}
	views, err := pg.ListViews(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	if views == nil {
		views = []pg.ViewInfo{}
	}
	matviews, err := pg.ListMatviews(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	if matviews == nil {
		matviews = []pg.ViewInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "views": views, "matviews": matviews})
}

func (s *Server) doPgViewCreate(a pgViewCreateArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.SQL) == "" {
		return "", fmt.Errorf("app, name y sql requeridos")
	}
	if a.Confirm != "create" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"create"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.CreateViewExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name), a.SQL, a.Materialized, a.OrReplace, a.WithData); err != nil {
		return "", err
	}
	kind := "vista"
	if a.Materialized {
		kind = "matview"
	}
	return fmt.Sprintf("%s %s creada", kind, strings.TrimSpace(a.Name)), nil
}

func (s *Server) doPgViewDrop(a pgViewDropArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app y name requeridos")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	v, err := pg.GetViewDef(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name))
	if err != nil {
		return "", err
	}
	if err := pg.DropViewExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name), v.Kind == "materialized"); err != nil {
		return "", err
	}
	return fmt.Sprintf("vista %s borrada", strings.TrimSpace(a.Name)), nil
}

func (s *Server) doPgMatviewRefresh(a pgMatviewRefreshArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app y name requeridos")
	}
	if a.Confirm != "refresh" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"refresh"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.RefreshMatviewExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name), a.WithData); err != nil {
		return "", err
	}
	return fmt.Sprintf("matview %s refrescada", strings.TrimSpace(a.Name)), nil
}

func (s *Server) doPgFunctions(a pgFunctionsArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if strings.TrimSpace(a.Func) != "" {
		f, err := pg.GetRoutineDef(ctx, cdb, rec.Schema, strings.TrimSpace(a.Func))
		if err != nil {
			return "", err
		}
		return jsonText(map[string]any{"app": a.App, "function": f})
	}
	funcs, err := pg.ListFunctions(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	if funcs == nil {
		funcs = []pg.FuncInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "functions": funcs})
}

func (s *Server) doPgFunctionDrop(a pgFunctionDropArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Func) == "" {
		return "", fmt.Errorf("app y func requeridos")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.DropRoutineExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Func), false); err != nil {
		return "", err
	}
	return fmt.Sprintf("función %s borrada", strings.TrimSpace(a.Func)), nil
}

func (s *Server) doPgProcedures(a pgProceduresArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if strings.TrimSpace(a.Procedure) != "" {
		p, err := pg.GetRoutineDef(ctx, cdb, rec.Schema, strings.TrimSpace(a.Procedure))
		if err != nil {
			return "", err
		}
		return jsonText(map[string]any{"app": a.App, "procedure": p})
	}
	procs, err := pg.ListProcedures(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	if procs == nil {
		procs = []pg.FuncInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "procedures": procs})
}

func (s *Server) doPgProcedureCall(a pgProcedureCallArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Procedure) == "" {
		return "", fmt.Errorf("app y procedure requeridos")
	}
	if a.Confirm != "call" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"call"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	res, err := pg.CallProcedureExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Procedure))
	if err != nil {
		return "", err
	}
	return jsonText(res)
}

func (s *Server) doPgProcedureDrop(a pgProcedureDropArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Procedure) == "" {
		return "", fmt.Errorf("app y procedure requeridos")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.DropRoutineExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Procedure), true); err != nil {
		return "", err
	}
	return fmt.Sprintf("procedimiento %s borrado", strings.TrimSpace(a.Procedure)), nil
}

func (s *Server) doPgTriggers(a pgTriggersArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	trs, err := pg.ListTriggers(ctx, cdb, rec.Schema, strings.TrimSpace(a.Table))
	if err != nil {
		return "", err
	}
	if trs == nil {
		trs = []pg.TriggerInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "table": strings.TrimSpace(a.Table), "triggers": trs})
}

func (s *Server) doPgTriggerCreate(a pgTriggerCreateArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Timing) == "" || strings.TrimSpace(a.Function) == "" {
		return "", fmt.Errorf("app, table, name, timing y function requeridos")
	}
	if len(a.Events) == 0 {
		return "", fmt.Errorf("events requerido (insert|update|delete|truncate)")
	}
	if a.Confirm != "create" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"create"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.CreateTriggerExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Table), strings.TrimSpace(a.Name), strings.TrimSpace(a.Timing), []string(a.Events), strings.TrimSpace(a.Orientation), strings.TrimSpace(a.Function), strings.TrimSpace(a.Condition)); err != nil {
		return "", err
	}
	return fmt.Sprintf("trigger %s creado en %s", strings.TrimSpace(a.Name), strings.TrimSpace(a.Table)), nil
}

func (s *Server) doPgTriggerDrop(a pgTriggerDropArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app, table y name requeridos")
	}
	if a.Confirm != "drop" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"drop"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.DropTriggerExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Table), strings.TrimSpace(a.Name)); err != nil {
		return "", err
	}
	return fmt.Sprintf("trigger %s borrado de %s", strings.TrimSpace(a.Name), strings.TrimSpace(a.Table)), nil
}

func (s *Server) doPgTriggerToggle(a pgTriggerToggleArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Table) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app, table y name requeridos")
	}
	if a.Confirm != "toggle" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"toggle"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	if err := pg.ToggleTriggerExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Table), strings.TrimSpace(a.Name), a.Enable); err != nil {
		return "", err
	}
	verb := "desactivado"
	if a.Enable {
		verb = "activado"
	}
	return fmt.Sprintf("trigger %s %s", strings.TrimSpace(a.Name), verb), nil
}

func (s *Server) doPgSequences(a pgSequencesArg) (string, error) {
	if strings.TrimSpace(a.App) == "" {
		return "", fmt.Errorf("app requerida")
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	seqs, err := pg.ListSequences(ctx, cdb, rec.Schema)
	if err != nil {
		return "", err
	}
	if seqs == nil {
		seqs = []pg.SeqInfo{}
	}
	return jsonText(map[string]any{"app": a.App, "sequences": seqs})
}

func (s *Server) doPgSequenceRestart(a pgSequenceRestartArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app y name requeridos")
	}
	if a.Confirm != "restart" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"restart"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	var with *int64
	if a.With != 0 {
		v := a.With
		with = &v
	}
	if err := pg.RestartSequenceExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name), with); err != nil {
		return "", err
	}
	return fmt.Sprintf("secuencia %s reiniciada", strings.TrimSpace(a.Name)), nil
}

func (s *Server) doPgSequenceSetval(a pgSequenceSetvalArg) (string, error) {
	if strings.TrimSpace(a.App) == "" || strings.TrimSpace(a.Name) == "" {
		return "", fmt.Errorf("app y name requeridos")
	}
	if a.Confirm != "setval" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"setval"}`)
	}
	rec, cfg, err := s.pgAppDB(a.App)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cdb, err := pg.Connect(ctx, pg.SuperDSN(cfg.Host, cfg.Port, rec.Role, "", cfg.DB))
	if err != nil {
		return "", err
	}
	defer cdb.Close()
	v, err := pg.SetvalExec(ctx, cdb, rec.Schema, strings.TrimSpace(a.Name), a.Value, a.IsCalled)
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"app": a.App, "name": strings.TrimSpace(a.Name), "value": v})
}

func (s *Server) doAppEnvKeys(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	keys := make([]string, 0, len(a.Env))
	for k := range a.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return jsonText(map[string]any{"app": app, "keys": keys})
}

func (s *Server) doEnvDelete(a envDelArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		app := cfgfile.FindApp(cp, a.App)
		if app == nil {
			return fmt.Errorf("app %q inexistente", a.App)
		}
		if _, ok := app.Env[a.Key]; !ok {
			return fmt.Errorf("%s no tiene %s", a.App, a.Key)
		}
		return cfgfile.UnsetEnv(cp, a.App, a.Key)
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %s borrada (aplica en ≤10s)", a.App, a.Key), nil
}

func (s *Server) doEnvGlobalList(a envGlobalListArg) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	keys := make([]string, 0, len(cfg.GlobalEnv))
	for k := range cfg.GlobalEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if a.IncludeSecrets {
		out := make(map[string]string, len(keys))
		for _, k := range keys {
			out[k] = cfg.GlobalEnv[k]
		}
		return jsonText(map[string]any{"keys": keys, "values": out, "count": len(keys)})
	}
	return jsonText(map[string]any{"keys": keys, "count": len(keys)})
}

func (s *Server) doEnvGlobalSet(a envGlobalArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := cfgfile.SetGlobalEnv(db, a.Key, a.Value); err != nil {
		return "", err
	}
	return fmt.Sprintf("global %s guardada (las apps la heredan al reiniciar)", a.Key), nil
}

func (s *Server) doEnvGlobalDelete(a envGlobalArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateInfra(db, func(cp *config.Config) error {
		if _, ok := cp.GlobalEnv[a.Key]; !ok {
			return fmt.Errorf("no existe la global %s", a.Key)
		}
		cfgfile.UnsetGlobalEnvValue(cp, a.Key)
		return nil
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("global %s eliminada", a.Key), nil
}

func (s *Server) doRoutesAdd(a routesAddArg) (string, error) {
	if len(a.Hosts) == 0 && a.Path == "" {
		return "", fmt.Errorf("faltan hosts o path")
	}
	if a.Target == "" {
		return "", fmt.Errorf("falta target")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		return cfgfile.AddRoute(cp, a.Hosts, a.Target, a.Path)
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("ruta %s → %s creada", strings.Join(a.Hosts, ","), a.Target), nil
}

func (s *Server) doRoutesUpdate(a routesUpdateArg) (string, error) {
	if a.Host == "" || a.Target == "" {
		return "", fmt.Errorf("faltan host/target")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	found := false
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		ok, err := cfgfile.SetRouteTarget(cp, a.Host, a.Target)
		found = ok
		return err
	}); err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("host %q no existe", a.Host)
	}
	return fmt.Sprintf("ruta %s → %s", a.Host, a.Target), nil
}

func (s *Server) doRoutesRemove(a hostArg) (string, error) {
	if a.Host == "" && a.Path == "" {
		return "", fmt.Errorf("falta host o path")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if a.Path == "" {
		removed := false
		if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
			ok, err := cfgfile.RemoveRouteHost(cp, a.Host)
			removed = ok
			return err
		}); err != nil {
			return "", err
		}
		if !removed {
			return "", fmt.Errorf("host %q no existe", a.Host)
		}
		return fmt.Sprintf("host %s quitado", a.Host), nil
	}
	dropped := 0
	if _, err := cfgfile.MutateFleet(db, func(cp *config.Config) error {
		n, err := cfgfile.RemoveRoutesFull(cp, a.Host, a.Path)
		dropped = n
		return err
	}); err != nil {
		return "", err
	}
	if dropped == 0 {
		return "", fmt.Errorf("ninguna ruta coincide")
	}
	return fmt.Sprintf("%d ruta(s) eliminada(s)", dropped), nil
}

func (s *Server) doDomainsSuggest(name string) (string, error) {
	slug := strings.Trim(regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		return "", fmt.Errorf("falta name")
	}
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	used := map[string]bool{}
	for _, r := range cfg.Routes {
		for _, h := range r.Hosts {
			used[strings.ToLower(h)] = true
		}
	}
	out := []map[string]string{}
	add := func(h, kind, note string) {
		if h == "" || used[strings.ToLower(h)] {
			return
		}
		out = append(out, map[string]string{"host": h, "kind": kind, "note": note})
	}
	add(slug+".localhost", "local", "funciona sin editar hosts")
	add(slug+".test", "local", "requiere hosts → 127.0.0.1")
	if base := strings.Trim(strings.TrimSpace(db.GetSetting(store.KeyBaseDomain)), "."); base != "" {
		add(slug+"."+base, "base", "tu dominio base")
	}
	return jsonText(map[string]any{"name": slug, "suggestions": out})
}

func (s *Server) doVerifyDomain(host string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if host == "" {
		return "", fmt.Errorf("falta host")
	}
	out := map[string]any{"host": host, "dns_ok": appview.ResolvesLocal(host)}
	if !cfg.Healthz.Enabled {
		out["error"] = "healthz desactivado"
		return jsonText(out)
	}
	// request al proxy HTTP por su listen local con Host: host
	addr := cfg.ListenHTTP
	if addr == "" {
		out["error"] = "listen_http vacío (modo solo TLS): usa el navegador"
		return jsonText(out)
	}
	addr = strings.TrimPrefix(addr, "0.0.0.0")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 4 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
	req.Host = host
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		out["http_error"] = err.Error()
		return jsonText(out)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	out["status"] = resp.StatusCode
	out["ms"] = time.Since(start).Milliseconds()
	out["body"] = string(body)
	return jsonText(out)
}

func (s *Server) doHostsList() (string, error) {
	list, err := hosts.List()
	if err != nil {
		return "", err
	}
	return jsonText(map[string]any{"path": hosts.Path(), "hosts": list})
}

func (s *Server) doHostsAdd(host string) (string, error) {
	if err := hosts.Add(host); err != nil {
		return "", fmt.Errorf("%v (requiere administrador)", err)
	}
	return fmt.Sprintf("%s → 127.0.0.1 en %s", host, hosts.Path()), nil
}

func (s *Server) doHostsRemove(host string) (string, error) {
	if err := hosts.Remove(host); err != nil {
		return "", fmt.Errorf("%v (requiere administrador)", err)
	}
	return fmt.Sprintf("%s quitado de hosts", host), nil
}

func (s *Server) doDeploymentGet(id int64) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	deps, err := db.ListDeployments("", 2000)
	if err != nil {
		return "", err
	}
	for _, d := range deps {
		if d.ID == id {
			return jsonText(d)
		}
	}
	return "", fmt.Errorf("deployment %d inexistente", id)
}

func (s *Server) doDeployWait(a deployWaitArg) (string, error) {
	timeout := a.TimeoutS
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	deadline := time.Now().Add(time.Duration(timeout) * time.Second)
	for {
		deps, err := db.ListDeployments(a.App, 50)
		if err != nil {
			return "", err
		}
		for _, d := range deps {
			if a.ID > 0 && d.ID != a.ID {
				continue
			}
			if a.ID == 0 && a.App != "" && d.App != a.App {
				continue
			}
			if d.Status != "running" {
				return jsonText(d)
			}
			break
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("deployment sin terminar tras %ds", timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *Server) doGitStatus(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	run := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", a.Dir}, args...)...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	dirty := run("status", "--porcelain")
	return jsonText(map[string]any{
		"app": app, "dir": a.Dir,
		"head":        run("rev-parse", "--short", "HEAD"),
		"branch":      run("branch", "--show-current"),
		"remote":      run("remote", "get-url", "origin"),
		"dirty_files": len(strings.Split(dirty, "\n")) - boolToInt(dirty == ""),
		"dirty":       dirty != "",
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) doDiffRev(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	prev := deploy.OtherSlot(a.Dir)
	if _, err := os.Stat(prev); err != nil {
		return "", fmt.Errorf("sin versión previa para %s", app)
	}
	prevRev := ""
	if out, err := exec.Command("git", "-C", prev, "rev-parse", "HEAD").Output(); err == nil {
		prevRev = strings.TrimSpace(string(out))
	}
	curRev := ""
	if out, err := exec.Command("git", "-C", a.Dir, "rev-parse", "HEAD").Output(); err == nil {
		curRev = strings.TrimSpace(string(out))
	}
	changed := ""
	if prevRev != "" && curRev != "" {
		if out, err := exec.Command("git", "-C", a.Dir, "diff", "--name-status", prevRev, curRev).Output(); err == nil {
			changed = string(out)
		}
	}
	return jsonText(map[string]any{"app": app, "prev": prevRev, "current": curRev, "changed": changed})
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *Server) doDiskUsage(app string) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	a := cfgfile.FindApp(cfg, app)
	if a == nil {
		return "", fmt.Errorf("app %q inexistente", app)
	}
	out := map[string]any{"app": app, "dir": a.Dir}
	slots := map[string]string{"live": a.Dir, "prev": deploy.OtherSlot(a.Dir)}
	for name, p := range slots {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out[name] = dirSize(p)
		}
	}
	return jsonText(out)
}

func (s *Server) doLogSearch(a logSearchArg) (string, error) {
	path := logx.Path(s.cfgPath)
	if a.App != "" {
		path = filepath.Join(logx.DirFor(s.cfgPath), "apps", a.App+".log")
	}
	txt, err := logx.Tail(path, 5000)
	if err != nil {
		return "", err
	}
	rx, err := regexp.Compile(a.Pattern)
	if err != nil {
		return "", fmt.Errorf("regex inválida: %w", err)
	}
	limit := a.N
	if limit <= 0 {
		limit = 50
	}
	matches := []string{}
	for _, line := range strings.Split(txt, "\n") {
		if rx.MatchString(line) {
			matches = append(matches, line)
		}
	}
	if len(matches) > limit {
		matches = matches[len(matches)-limit:]
	}
	return strings.Join(matches, "\n"), nil
}

func (s *Server) doUsageSummary() (string, error) {
	entries := weblog.ParseFile(logx.AccessPath(s.cfgPath))
	return jsonText(weblog.Summarize(entries))
}

func (s *Server) doProxyMap() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	out := []map[string]any{}
	for _, r := range cfg.Routes {
		rs := map[string]any{"hosts": r.Hosts, "target": r.Target}
		if u, err := url.Parse(r.Target); err == nil && u.Host != "" {
			rs["ok"] = health.BackendOK(u.Host)
		}
		dns := []bool{}
		for _, h := range r.Hosts {
			dns = append(dns, appview.ResolvesLocal(h))
		}
		rs["dns_ok"] = dns
		out = append(out, rs)
	}
	return jsonText(out)
}

func (s *Server) doBackendsProbe() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	seen := map[string]bool{}
	out := []map[string]any{}
	for _, r := range cfg.Routes {
		u, err := url.Parse(r.Target)
		if err != nil || u.Host == "" || seen[u.Host] {
			continue
		}
		seen[u.Host] = true
		out = append(out, map[string]any{"target": r.Target, "ok": health.BackendOK(u.Host)})
	}
	return jsonText(out)
}

func (s *Server) doSettingsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	out := map[string]string{}
	for _, k := range store.KnownSettings() {
		out[k] = settings.DisplayValue(k, db.GetSetting(k))
	}
	return jsonText(out)
}

func (s *Server) doSettingsSet(a settingsSetArg) (string, error) {
	known := false
	for _, k := range store.KnownSettings() {
		if k == a.Key {
			known = true
		}
	}
	if !known {
		return "", fmt.Errorf("setting desconocido: %s", a.Key)
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if eff, ok := settings.AcceptValue(a.Key, a.Value); !ok {
		return "sin cambios (valor enmascarado)", nil
	} else {
		a.Value = eff
	}
	if err := db.SetSetting(a.Key, a.Value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s actualizado", a.Key), nil
}

func (s *Server) doAlertsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rules, err := db.ListRules()
	if err != nil {
		return "", err
	}
	return jsonText(rules)
}

func (s *Server) doAlertsAdd(a alertAddArg) (string, error) {
	app := a.App
	if app == "" {
		app = "*"
	}
	kind := a.Kind
	if kind == "" {
		kind = "failed"
	}
	th := a.Threshold
	if th == 0 {
		th = 1
	}
	win := a.WindowMin
	if win <= 0 {
		win = 60
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	id, err := db.AddRule(app, kind, th, win)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("regla #%d creada (%s %s)", id, app, kind), nil
}

func (s *Server) doAlertsRemove(id int64) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.DeleteRule(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("regla #%d borrada", id), nil
}

func (s *Server) doAlertsUpdate(a alertUpdateArg) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rules, err := db.ListRules()
	if err != nil {
		return "", err
	}
	var rule *store.AlertRule
	for i := range rules {
		if rules[i].ID == a.ID {
			rule = &rules[i]
		}
	}
	if rule == nil {
		return "", fmt.Errorf("regla #%d inexistente", a.ID)
	}
	app, kind := rule.App, rule.Kind
	th, win := rule.Threshold, rule.WindowMin
	enabled, channel := rule.Enabled, rule.ChannelID
	if a.App != "" {
		app = a.App
	}
	if a.Kind != "" {
		kind = a.Kind
	}
	if a.Threshold != nil {
		th = *a.Threshold
	}
	if a.WindowMin != nil {
		win = *a.WindowMin
	}
	if a.Enabled != nil {
		enabled = *a.Enabled
	}
	if a.ChannelID != nil {
		channel = *a.ChannelID
	}
	if err := db.UpdateRule(a.ID, app, kind, th, win, enabled, channel); err != nil {
		return "", err
	}
	return fmt.Sprintf("regla #%d actualizada", a.ID), nil
}

func (s *Server) doAlertsTest(id int64) (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	rules, err := db.ListRules()
	if err != nil {
		return "", err
	}
	for _, rule := range rules {
		if rule.ID == id {
			return alerts.Describe(rule, alerts.Metrics{}), nil
		}
	}
	return "", fmt.Errorf("regla #%d inexistente", id)
}

func (s *Server) doAlertChannelsList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	chans, err := db.ListChannels()
	if err != nil {
		return "", err
	}
	return jsonText(chans)
}

func (s *Server) doDiscoverStatus() (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	out := map[string]any{"configured": cfg.Github != nil}
	if cfg.Github != nil {
		keyOK := false
		if _, err := os.Stat(cfg.Github.KeyFile); err == nil {
			keyOK = true
		}
		out["app_slug"] = cfg.Github.AppSlug
		out["poll_interval"] = cfg.Github.PollInterval
		out["key_file_ok"] = keyOK
		out["webhook_secret"] = cfg.Github.WebhookSecret != ""
		out["account"] = db.GetSetting("github_account")
		out["repository_selection"] = db.GetSetting("github_repo_selection")
		out["repositories"] = db.GetSetting("github_repo_count")
		out["install_url"] = db.GetSetting("github_install_url")
		out["all_repositories"] = db.GetSetting("github_repo_selection") == "all"
	}
	out["base_domain"] = db.GetSetting(store.KeyBaseDomain)
	return jsonText(out)
}

func validRole(r string) bool {
	return r == store.RoleViewer || r == store.RoleOperator || r == store.RoleAdmin
}

func (s *Server) doUsersList() (string, error) {
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	users, err := db.ListUsers()
	if err != nil {
		return "", err
	}
	return jsonText(users)
}

func (s *Server) doUsersAdd(a userAddArg) (string, error) {
	if a.Name == "" || len(a.Password) < 8 {
		return "", fmt.Errorf("usuario vacío o contraseña <8")
	}
	role := a.Role
	if role == "" {
		role = store.RoleViewer
	}
	if !validRole(role) {
		return "", fmt.Errorf("rol inválido: %s", role)
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.CreateUser(a.Name, a.Password, role); err != nil {
		return "", err
	}
	return fmt.Sprintf("usuario %s (%s) creado", a.Name, role), nil
}

func (s *Server) doUsersRemove(a userArg) (string, error) {
	if a.Name == "" {
		return "", fmt.Errorf("falta name")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.DeleteUser(a.Name); err != nil {
		return "", err
	}
	return fmt.Sprintf("usuario %s borrado", a.Name), nil
}

func (s *Server) doUsersPasswd(a userPassArg) (string, error) {
	if a.Name == "" || len(a.Password) < 8 {
		return "", fmt.Errorf("usuario vacío o contraseña <8")
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := db.SetPassword(a.Name, a.Password); err != nil {
		return "", err
	}
	return fmt.Sprintf("password de %s actualizado (sesiones invalidadas)", a.Name), nil
}

func (s *Server) doConfigExport(a exportArg) (string, error) {
	cfg, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	settings := map[string]string{}
	for _, k := range store.KnownSettings() {
		if v := db.GetSetting(k); v != "" {
			settings[k] = v
		}
	}
	export := config.ExportJSON
	if a.IncludeSecrets {
		export = config.ExportJSONWithSecrets
	}
	out, err := export(cfg, settings)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *Server) doConfigImport(a importArg) (string, error) {
	if a.Confirm != "replace" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"replace"}`)
	}
	prev, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	cfg, settings, err := config.ImportJSONMerged([]byte(a.JSON), prev)
	if err != nil {
		return "", err
	}
	if err := cfgfile.ReplaceAll(db, cfg, settings); err != nil {
		return "", err
	}
	return fmt.Sprintf("config reemplazada: %d apps, %d rutas", len(cfg.Apps), len(cfg.Routes)), nil
}

func (s *Server) doPrune(a pruneArg) (string, error) {
	if a.Confirm != "prune" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"prune"}`)
	}
	depDays, evDays := a.DeploymentsDays, a.EventsDays
	if depDays <= 0 {
		depDays = 30
	}
	if evDays <= 0 {
		evDays = 30
	}
	_, db, err := s.load()
	if err != nil {
		return "", err
	}
	defer db.Close()
	deps, err := db.PruneDeployments(time.Now().AddDate(0, 0, -depDays))
	if err != nil {
		return "", err
	}
	evs, err := db.PruneEvents(time.Now().AddDate(0, 0, -evDays))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("podado: %d deployments, %d eventos", deps, evs), nil
}

// doLogsClear vacía los logs operativos (ver logx.ClearLogs). Destructivo:
// exige confirm=clear. No toca la DB (auditoría intacta).
func (s *Server) doLogsClear(a logsClearArg) (string, error) {
	if a.Confirm != "clear" {
		return "", fmt.Errorf(`confirmación requerida: {"confirm":"clear"}`)
	}
	cleared, err := logx.ClearLogs(s.cfgPath)
	if err != nil {
		return "", err
	}
	var total int64
	for _, b := range cleared {
		total += b
	}
	return jsonText(map[string]any{
		"cleared": cleared, "files": len(cleared), "bytes": total,
	})
}

// registerMore añade el catálogo extendido al servidor MCP.
func (s *Server) registerMore(srv *mcp.Server) {
	add := func(name, desc string, fn func() (string, error)) {
		addTool(s, srv, &mcp.Tool{Name: name, Description: desc},
			func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
				return text(fn())
			})
	}
	// tool con argumento app
	addTool(s, srv, &mcp.Tool{Name: "app_get", Description: "Detalle de una app (dominio, puerto, estático, deploy, dns_ok)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppGet(a.App))
		})
	addTool(s, srv, &mcp.Tool{Name: "app_env_keys", Description: "Nombres de variables de entorno (nunca valores)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAppEnvKeys(a.App))
		})
	addTool(s, srv, &mcp.Tool{Name: "env_delete", Description: "Borra una variable de entorno de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvDelete(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "env_global_list", Description: "Variables globales de todas las apps (claves; valores solo con include_secrets)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envGlobalListArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvGlobalList(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "env_global_set", Description: "Crea/actualiza una variable global; todas las apps la heredan al reiniciar."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envGlobalArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvGlobalSet(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "env_global_delete", Description: "Elimina una variable global de todas las apps."},
		func(ctx context.Context, req *mcp.CallToolRequest, a envGlobalArg) (*mcp.CallToolResult, any, error) {
			return text(s.doEnvGlobalDelete(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "routes_add", Description: "Crea una ruta: hosts (dominio) y/o path (/app) → target."},
		func(ctx context.Context, req *mcp.CallToolRequest, a routesAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesAdd(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "routes_update", Description: "Cambia el target de la ruta que contiene host."},
		func(ctx context.Context, req *mcp.CallToolRequest, a routesUpdateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesUpdate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "routes_remove", Description: "Quita un host de sus rutas, o rutas enteras por path (sin host)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a hostArg) (*mcp.CallToolResult, any, error) {
			return text(s.doRoutesRemove(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "domains_suggest", Description: "Dominios libres para un nombre (localhost/test/base)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDomainsSuggest(a.App))
		})
	addTool(s, srv, &mcp.Tool{Name: "verify_domain", Description: "Verifica DNS local y respuesta HTTP del proxy para un host."},
		func(ctx context.Context, req *mcp.CallToolRequest, a verifyArg) (*mcp.CallToolResult, any, error) {
			return text(s.doVerifyDomain(a.Host))
		})
	add("hosts_list", "Hosts gestionados por rkt en el archivo hosts.", s.doHostsList)
	addTool(s, srv, &mcp.Tool{Name: "hosts_add", Description: "Añade host → 127.0.0.1 al archivo hosts (admin)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a hostArg) (*mcp.CallToolResult, any, error) {
			return text(s.doHostsAdd(a.Host))
		})
	addTool(s, srv, &mcp.Tool{Name: "hosts_remove", Description: "Quita un host del bloque de rkt en hosts (admin)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a hostArg) (*mcp.CallToolResult, any, error) {
			return text(s.doHostsRemove(a.Host))
		})
	addTool(s, srv, &mcp.Tool{Name: "deployment_get", Description: "Estado de un deployment por id."},
		func(ctx context.Context, req *mcp.CallToolRequest, a deployWaitArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeploymentGet(a.ID))
		})
	addTool(s, srv, &mcp.Tool{Name: "deploy_wait", Description: "Espera a que un deployment termine (running → ok/failed)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a deployWaitArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDeployWait(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "git_status", Description: "HEAD, rama, remote y suciedad del dir live de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doGitStatus(a.App))
		})
	addTool(s, srv, &mcp.Tool{Name: "diff_rev", Description: "Archivos cambiados entre live.prev y live."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDiffRev(a.App))
		})
	addTool(s, srv, &mcp.Tool{Name: "disk_usage", Description: "Bytes de live/prev/staging de una app."},
		func(ctx context.Context, req *mcp.CallToolRequest, a nameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doDiskUsage(a.App))
		})
	addTool(s, srv, &mcp.Tool{Name: "log_search", Description: "Busca regex en logs (app u operativo), últimas coincidencias."},
		func(ctx context.Context, req *mcp.CallToolRequest, a logSearchArg) (*mcp.CallToolResult, any, error) {
			return text(s.doLogSearch(a))
		})
	add("usage_summary", "Resumen de tráfico desde access.log (total, bytes, clases, días, top).", s.doUsageSummary)
	add("proxy_map", "Rutas con backend ok y dns_ok (vista /proxy).", s.doProxyMap)
	add("backends_probe", "Dial TCP a cada backend único de las rutas.", s.doBackendsProbe)
	add("settings_list", "Claves de configuración y sus valores.", s.doSettingsList)
	addTool(s, srv, &mcp.Tool{Name: "settings_set", Description: "Cambia una clave de configuración (base_domain, node_bin, quotas…)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a settingsSetArg) (*mcp.CallToolResult, any, error) {
			return text(s.doSettingsSet(a))
		})
	add("alerts_list", "Reglas de alerta configuradas.", s.doAlertsList)
	addTool(s, srv, &mcp.Tool{Name: "alerts_add", Description: "Crea regla de alerta (failed|error_rate)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsAdd(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "alerts_remove", Description: "Borra una regla de alerta por id."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsRemove(a.ID))
		})
	addTool(s, srv, &mcp.Tool{Name: "alerts_update", Description: "Edita una regla de alerta (app, kind, umbral, ventana, enabled, canal)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertUpdateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsUpdate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "alerts_test", Description: "Describe el disparo de una regla sin ejecutarlo."},
		func(ctx context.Context, req *mcp.CallToolRequest, a alertDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doAlertsTest(a.ID))
		})
	add("alert_channels_list", "Canales webhook configurados.", s.doAlertChannelsList)
	add("discover_status", "Estado de GitHub App/base_domain para auto-registro.", s.doDiscoverStatus)
	add("users_list", "Usuarios (sin hashes) y roles.", s.doUsersList)
	addTool(s, srv, &mcp.Tool{Name: "users_add", Description: "Crea usuario local (password ≥12). Queda en el historial del agente."},
		func(ctx context.Context, req *mcp.CallToolRequest, a userAddArg) (*mcp.CallToolResult, any, error) {
			return text(s.doUsersAdd(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "users_remove", Description: "Borra un usuario local."},
		func(ctx context.Context, req *mcp.CallToolRequest, a userArg) (*mcp.CallToolResult, any, error) {
			return text(s.doUsersRemove(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "users_passwd", Description: "Cambia password de un usuario (password ≥12)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a userPassArg) (*mcp.CallToolResult, any, error) {
			return text(s.doUsersPasswd(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "config_export", Description: "Exporta configuración y apps en JSON (secretos solo con include_secrets)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a exportArg) (*mcp.CallToolResult, any, error) {
			return text(s.doConfigExport(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "config_import", Description: "Reemplaza configuración y apps desde JSON (confirm=replace). Destructivo."},
		func(ctx context.Context, req *mcp.CallToolRequest, a importArg) (*mcp.CallToolResult, any, error) {
			return text(s.doConfigImport(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "prune", Description: "Poda deployments/events viejos (confirm=prune)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pruneArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPrune(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "logs_clear", Description: "Vacía rkt.log, access.log, apps/*.log y deploys/*.log + backups .1 (confirm=clear). No toca la DB."},
		func(ctx context.Context, req *mcp.CallToolRequest, a logsClearArg) (*mcp.CallToolResult, any, error) {
			return text(s.doLogsClear(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_status", Description: "Estado del Postgres gestionado: versión y base (sin secretos). Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doPgStatus())
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_list", Description: "Bases por app: esquema, rol, modo y DATABASE_URL (sin password). Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
			return text(s.doPgList())
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_provision", Description: "Crea esquema+rol para una app e inyecta DATABASE_URL en su env (mode dedicated|shared, default dedicated)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgProvisionArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgProvision(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_drop", Description: "Borra esquema+rol de una app (confirm=drop). Destructivo."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgDropArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgDrop(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_backup", Description: "Vuelca el esquema de una app a data/backups (formato custom)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgAppArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgBackup(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_tables", Description: "Lista tablas del esquema de una app. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgTablesArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgTables(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_rows", Description: "Pagina una tabla del esquema (n/page, order/dir, filters col:op:val, search). Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgRowsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgRows(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_schema", Description: "Inspector de tabla (columnas, tipos, PK, RLS, conteo) o de todas. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgSchemaArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgSchema(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_insert", Description: "Inserta una fila en una tabla (confirm=insert)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgInsertArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgInsert(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_update", Description: "Actualiza una fila por PK exacta (confirm=update)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgUpdateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgUpdate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_delete", Description: "Borra filas por PK o filtros (confirm=delete). Sin pk ni filtros no borra."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgDeleteArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgDelete(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_query", Description: "Ejecuta una sentencia SQL en el esquema de la app (confirm=query). SELECT devuelve filas; el resto, afectadas."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgQueryArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgQuery(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_restore", Description: "Restaura un dump custom (-Fc) sobre la app (confirm=restore). Sobrescribe datos."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgRestoreArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgRestore(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_indexes", Description: "Lista índices de una tabla. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgIndexArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgIndexes(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_create_index", Description: "Crea un índice en una tabla (confirm=create)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgCreateIndexArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgCreateIndex(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_drop_index", Description: "Borra un índice del esquema (confirm=drop)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgDropIndexArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgDropIndex(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_constraints", Description: "Lista constraints (PK, FK, unique, check) de una tabla. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgConstraintsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgConstraints(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_policy_create", Description: "Crea una policy RLS en una tabla (confirm=create)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgPolicyCreateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgPolicyCreate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_policy_drop", Description: "Borra una policy RLS de una tabla (confirm=drop)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgPolicyDropArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgPolicyDrop(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_rename_table", Description: "Renombra una tabla del esquema (confirm=alter)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgRenameArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgRenameTable(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_rename_column", Description: "Renombra una columna de una tabla (confirm=alter)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgRenameColArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgRenameColumn(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_alter_column", Description: "Altera tipo, DEFAULT o NOT NULL de una columna (confirm=alter)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgAlterColArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgAlterColumn(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_truncate", Description: "Vacía una tabla sin borrar su esquema (confirm=truncate). Destructivo."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgTruncateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgTruncate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_duplicate", Description: "Duplica una tabla en otra nueva, con o sin datos (confirm=duplicate)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgDuplicateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgDuplicate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_backups", Description: "Lista backups del esquema de una app. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgBackupsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgBackups(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_backup_delete", Description: "Borra un backup del disco (confirm=delete)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgBackupDelArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgBackupDelete(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_rows_import", Description: "Inserta un lote de filas en una tabla (confirm=import, máx 1000 filas)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgImportArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgImport(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_query_batch", Description: "Ejecuta un lote SQL de hasta 20 sentencias (confirm=batch)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgBatchArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgBatch(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_query_explain", Description: "Muestra el plan EXPLAIN de una sentencia (confirm=explain)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgExplainArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgExplain(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_views", Description: "Lista views y matviews del esquema, o su definición si indicas name. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgViewsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgViews(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_view_create", Description: "Crea una vista o matview desde un SELECT (confirm=create)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgViewCreateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgViewCreate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_view_drop", Description: "Borra una vista o matview del esquema (confirm=drop)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgViewDropArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgViewDrop(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_matview_refresh", Description: "Refresca una matview con o sin datos (confirm=refresh)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgMatviewRefreshArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgMatviewRefresh(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_functions", Description: "Lista funciones del esquema, o su definición si indicas func. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgFunctionsArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgFunctions(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_function_drop", Description: "Borra una función por identidad nombre(args) (confirm=drop)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgFunctionDropArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgFunctionDrop(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_procedures", Description: "Lista procedimientos del esquema, o su definición si indicas procedure. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgProceduresArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgProcedures(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_procedure_call", Description: "Ejecuta CALL a un procedimiento con literales (confirm=call)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgProcedureCallArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgProcedureCall(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_procedure_drop", Description: "Borra un procedimiento por identidad nombre(args) (confirm=drop)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgProcedureDropArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgProcedureDrop(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_triggers", Description: "Lista triggers del esquema, de una tabla si indicas table. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgTriggersArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgTriggers(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_trigger_create", Description: "Crea un trigger en una tabla con eventos y función (confirm=create)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgTriggerCreateArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgTriggerCreate(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_trigger_drop", Description: "Borra un trigger de una tabla (confirm=drop)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgTriggerDropArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgTriggerDrop(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_trigger_toggle", Description: "Activa o desactiva un trigger de una tabla (confirm=toggle)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgTriggerToggleArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgTriggerToggle(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_sequences", Description: "Lista secuencias del esquema con su estado. Solo lectura."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgSequencesArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgSequences(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_sequence_restart", Description: "Reinicia una secuencia, con valor opcional (confirm=restart)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgSequenceRestartArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgSequenceRestart(a))
		})
	addTool(s, srv, &mcp.Tool{Name: "pg_sequence_setval", Description: "Fija el valor de una secuencia y devuelve el nuevo (confirm=setval)."},
		func(ctx context.Context, req *mcp.CallToolRequest, a pgSequenceSetvalArg) (*mcp.CallToolResult, any, error) {
			return text(s.doPgSequenceSetval(a))
		})
	s.registerMonitor(srv)
}
