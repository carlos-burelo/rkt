package pg

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Record mapeo app→esquema (lo persiste el caller vía Registry).
type Record struct {
	App    string
	Schema string
	Role   string
	Mode   string
	DB     string
}

// Registry persistencia mínima que el caller aporta (store.DB la cumple
// con adaptadores de una línea). Interfaz en vez de *store.DB para no
// acoplar pg al storage y poder probar con fakes en memoria.
type Registry interface {
	PgGet(app string) (Record, bool, error)
	PgList() ([]Record, error)
	PgPut(app, schema, role, mode, db string) error
	PgRemove(app string) error
}

// Config conexión de superusuario local. Host siempre loopback
// (invariante de seguridad: las reglas trust solo cubren loopback).
type Config struct {
	Host string
	Port int
	User string
	Pass string
	DB   string
	Bins Bins
}

// LoadConfig lee settings (get = db.GetSetting). pg_mode debe ser local.
func LoadConfig(get func(string) string) (Config, error) {
	if strings.TrimSpace(get("pg_mode")) != "local" {
		return Config{}, fmt.Errorf("pg desactivado (settings set pg_mode local)")
	}
	port := DefaultPort
	if v := strings.TrimSpace(get("pg_port")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("pg_port inválido: %s", v)
		}
		port = n
	}
	db := strings.TrimSpace(get("pg_database"))
	if db == "" {
		db = DefaultDatabase
	}
	if err := ValidIdent(db); err != nil {
		return Config{}, fmt.Errorf("pg_database: %w", err)
	}
	user := strings.TrimSpace(get("pg_user"))
	if user == "" {
		user = "postgres"
	}
	bins, err := ResolveBin(strings.TrimSpace(get("pg_bin")))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Host: "127.0.0.1", Port: port, User: user,
		Pass: get("pg_pass"), DB: db, Bins: bins,
	}, nil
}

// super conecta como superusuario a una DB concreta.
func super(ctx context.Context, cfg Config, db string) (*sql.DB, error) {
	return Connect(ctx, SuperDSN(cfg.Host, cfg.Port, cfg.User, cfg.Pass, db))
}

// ProvisionApp crea esquema+rol+grants, regla trust en pg_hba, registra
// en el Registry y devuelve la DATABASE_URL (sin password: trust por
// rol+origen). Idempotente ante reintentos (el registro bloquea duplicados).
func ProvisionApp(ctx context.Context, reg Registry, cfg Config, app, mode string) (string, error) {
	if _, ok, err := reg.PgGet(app); err != nil {
		return "", err
	} else if ok {
		return "", fmt.Errorf("pg: %q ya tiene esquema", app)
	}
	if mode == "" {
		mode = "dedicated"
	}
	p, err := PlanProvision(app, mode, cfg.DB, cfg.Host, cfg.Port)
	if err != nil {
		return "", err
	}
	adb, err := super(ctx, cfg, "postgres")
	if err != nil {
		return "", err
	}
	defer adb.Close()
	if err := EnsureDatabase(ctx, adb, cfg.DB); err != nil {
		return "", err
	}
	tgt, err := super(ctx, cfg, cfg.DB)
	if err != nil {
		return "", err
	}
	defer tgt.Close()
	if err := ApplyProvision(ctx, tgt, p); err != nil {
		return "", err
	}
	if p.Mode == "shared" {
		// Un ALTER por (rol, db): recalcula con todos los esquemas vigentes.
		recs, err := reg.PgList()
		if err != nil {
			return "", err
		}
		schemas := []string{p.Schema}
		for _, r := range recs {
			if r.Mode == "shared" && r.Schema != p.Schema {
				schemas = append(schemas, r.Schema)
			}
		}
		if _, err := tgt.ExecContext(ctx, SharedSearchPathStmt(cfg.DB, schemas)); err != nil {
			return "", fmt.Errorf("search_path compartido: %w", err)
		}
	}
	// trustRoles roles con acceso: los ya registrados más el nuevo.
	recs, err := reg.PgList()
	if err != nil {
		return "", err
	}
	roles := []string{p.Role}
	for _, r := range recs {
		roles = append(roles, r.Role)
	}
	if err := refreshHBA(ctx, tgt, cfg.DB, roles); err != nil {
		return "", err
	}
	if err := reg.PgPut(app, p.Schema, p.Role, p.Mode, p.DB); err != nil {
		return "", err
	}
	return p.URL, nil
}

// refreshHBA reescribe el bloque gestionado y recarga sin reiniciar.
func refreshHBA(ctx context.Context, tgt *sql.DB, db string, roles []string) error {
	var hba string
	if err := tgt.QueryRowContext(ctx, `SHOW hba_file`).Scan(&hba); err != nil {
		return fmt.Errorf("hba_file: %w", err)
	}
	if _, err := ApplyHBAFile(hba, ManagedHBA(db, roles)); err != nil {
		return err
	}
	return ReloadConf(ctx, tgt)
}

// DropApp borra esquema+rol, limpia su regla hba y el registro.
func DropApp(ctx context.Context, reg Registry, cfg Config, app string) error {
	rec, ok, err := reg.PgGet(app)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pg: %q sin esquema", app)
	}
	tgt, err := super(ctx, cfg, cfg.DB)
	if err != nil {
		return err
	}
	defer tgt.Close()
	if err := DropAll(ctx, tgt, rec.Schema, rec.Role, rec.Mode); err != nil {
		return err
	}
	recs, err := reg.PgList()
	if err != nil {
		return err
	}
	var roles, sharedSchemas []string
	for _, r := range recs {
		if r.Role != rec.Role {
			roles = append(roles, r.Role)
		}
		if r.Mode == "shared" && r.Schema != rec.Schema {
			sharedSchemas = append(sharedSchemas, r.Schema)
		}
	}
	if rec.Mode == "shared" {
		if _, err := tgt.ExecContext(ctx, SharedSearchPathStmt(cfg.DB, sharedSchemas)); err != nil {
			return fmt.Errorf("search_path compartido: %w", err)
		}
	}
	if err := refreshHBA(ctx, tgt, cfg.DB, roles); err != nil {
		return err
	}
	return reg.PgRemove(app)
}

// BackupApp vuelca el esquema en formato custom (-Fc) a outDir con
// nombre pg-<app>-<timestamp>.dump. Devuelve la ruta.
func BackupApp(ctx context.Context, cfg Config, reg Registry, app, outDir string) (string, error) {
	rec, ok, err := reg.PgGet(app)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("pg: %q sin esquema", app)
	}
	ts := time.Now().Format("20060102-150405")
	out := filepath.Join(outDir, fmt.Sprintf("pg-%s-%s.dump", app, ts))
	if err := BackupTo(ctx, cfg.Bins.PgDump, cfg.Host, cfg.Port, cfg.User, cfg.Pass, rec.DB, rec.Schema, out); err != nil {
		return "", err
	}
	return out, nil
}
