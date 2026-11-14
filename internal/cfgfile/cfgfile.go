// Package cfgfile: configuración en SQLite (única fuente). El CLI muta
// por aquí; el servicio solo lee.
package cfgfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/platform"
	"rkt-proxy/internal/store"
)

// Load infra desde DB (sin flota). Para CLI que solo necesita healthz.
func Load(path string) (*config.Config, error) {
	db, err := store.Open(logx.DBPath(path))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return LoadBase(db, path)
}

// LoadMerged abre DB, carga infra + flota y la devuelve lista.
func LoadMerged(cfgPath string) (*config.Config, *store.DB, error) {
	if err := MigrateLegacyHome(cfgPath); err != nil {
		return nil, nil, err
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		return nil, nil, err
	}
	base, err := LoadBase(db, cfgPath)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	merged, err := Snapshot(db, base)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	if err := OverlayGithub(merged, db); err != nil {
		db.Close()
		return nil, nil, err
	}
	return merged, db, nil
}

// MigrateLegacyHome mueve DB/backups/certs del layout anterior al hogar
// único. Solo cuando hay DB legacy (upgrade real): en fresco no toca nada
// ajeno (ni ProgramData de otra instalación ni TempDirs de tests).
// Idempotente por destino: nunca pisa. Las apps conservan sus Dir
// absolutos, así que no se tocan. Falla en voz alta antes que arrancar
// con una DB vacía teniendo datos viejos (el servicio debe estar detenido;
// un CLI concurrente también bloquea el move en Windows).
func MigrateLegacyHome(cfgPath string) error {
	legacyDB := filepath.Join(logx.DirFor(cfgPath), "rkt.db")
	if _, err := os.Stat(legacyDB); err != nil {
		return nil
	}
	home := logx.HomeDir(cfgPath)
	data := logx.DataDir(cfgPath)
	// DB + journal/wal (shm es caché: se reconstruye solo, no se migra).
	newDB := filepath.Join(data, "rkt.db")
	for _, s := range []string{"", "-wal", "-journal"} {
		if err := moveFileIfAbsent(legacyDB+s, newDB+s); err != nil {
			return err
		}
	}
	// Backups diarios.
	if _, err := os.Stat(filepath.Join(data, "backups")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(logx.DirFor(cfgPath), "backups")); err == nil {
			if err := moveDir(filepath.Join(logx.DirFor(cfgPath), "backups"),
				filepath.Join(data, "backups")); err != nil {
				return err
			}
		}
	}
	// Certs: el default viejo (ProgramData) al home. Mover, no re-emitir:
	// re-emitir todo de golpe puede chocar con rate limits de Let's Encrypt.
	if dst := platform.CertsDir(home); dst != platform.DefaultCertsDir() {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			if _, err := os.Stat(platform.DefaultCertsDir()); err == nil {
				if err := moveDir(platform.DefaultCertsDir(), dst); err != nil {
					return fmt.Errorf("mueve certs a %s a mano: %w", dst, err)
				}
			}
		}
	}
	return nil
}

// moveFileIfAbsent renombra un fichero solo si el destino no existe y el
// origen sí. Falla en voz alta (nunca arrancar vacío por un move a medias).
func moveFileIfAbsent(old, new string) error {
	if _, err := os.Stat(new); err == nil {
		return nil
	}
	if _, err := os.Stat(old); err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(new), 0o755); err != nil {
		return err
	}
	if err := os.Rename(old, new); err != nil {
		return fmt.Errorf("mueve %s a %s a mano (detén rkt antes): %w", old, new, err)
	}
	return nil
}

// moveDir mueve un árbol; si Rename falla (otro volumen), copia y borra.
func moveDir(old, new string) error {
	if err := os.MkdirAll(filepath.Dir(new), 0o755); err != nil {
		return err
	}
	if err := os.Rename(old, new); err == nil {
		return nil
	}
	if err := copyDir(old, new); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

// copyDir replica un árbol (ficheros a 0o600: secretos y backups).
func copyDir(old, new string) error {
	return filepath.WalkDir(old, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(old, p)
		dst := filepath.Join(new, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o600)
	})
}

// No hay archivo de configuración: SQLite es la única fuente.
func LoadBase(db *store.DB, cfgPath string) (*config.Config, error) {
	raw, err := db.LoadInfra()
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		base := config.Default()
		if err := json.Unmarshal(raw, base); err != nil {
			return nil, fmt.Errorf("infra corrupta en DB: %w", err)
		}
		applyListenerSettings(base, db)
		applyPublicHTTPSDefault(base, db)
		if err := base.Finalize(); err != nil {
			return nil, err
		}
		return base, nil
	}

	base := config.Default()
	applyListenerSettings(base, db)
	applyPublicHTTPSDefault(base, db)
	if err := base.Finalize(); err != nil {
		return nil, err
	}
	if err := SaveInfra(db, base); err != nil {
		return nil, err
	}
	return base, nil
}

// applyListenerSettings permite cambiar listeners desde el CLI/API sin
// editar JSON. "off" desactiva el listener (útil para listen_https).
func applyListenerSettings(cfg *config.Config, db *store.DB) {
	if v := strings.TrimSpace(db.GetSetting(store.KeyListenHTTP)); v != "" {
		if v == "off" {
			v = ""
		}
		cfg.ListenHTTP = v
	}
	if v := strings.TrimSpace(db.GetSetting(store.KeyListenHTTPS)); v != "" {
		if v == "off" {
			v = ""
		}
		cfg.ListenHTTPS = v
	}
}

// applyPublicHTTPSDefault activa :443 cuando el servidor tiene IP pública y
// el usuario no fijó listen_https. "off" lo desactiva explícito y manda.
// No se persiste: se calcula en cada arranque, así la misma DB sirve en
// local (HTTP) y en servidor (HTTPS). Requiere reinicio para aplicar.
func applyPublicHTTPSDefault(cfg *config.Config, db *store.DB) {
	if strings.TrimSpace(db.GetSetting(store.KeyListenHTTPS)) != "" {
		return
	}
	if strings.TrimSpace(cfg.ListenHTTPS) != "" {
		return
	}
	if strings.TrimSpace(db.GetSetting(store.KeyPublicIP)) == "" {
		return
	}
	cfg.ListenHTTPS = ":443"
}

// SaveInfra persiste la parte de infraestructura (apps/rutas no van aquí).
func SaveInfra(db *store.DB, cfg *config.Config) error {
	cfg.ApplyDefaults()
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return db.SaveInfra(raw)
}

// ApplyDev impone los listeners de `rkt dev` en cada arranque y activa la CA
// local cuando hay HTTPS. La DB sigue mandando en todo lo demás; esto evita
// que --http/--https se ignoren cuando la infra ya existe.
func ApplyDev(db *store.DB, httpAddr, httpsAddr string) error {
	cfg := config.Default()
	raw, err := db.LoadInfra()
	if err != nil {
		return err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, cfg); err != nil {
			return fmt.Errorf("infra corrupta en DB: %w", err)
		}
	}
	cfg.ListenHTTP = httpAddr
	cfg.ListenHTTPS = httpsAddr
	cfg.LocalCA = httpsAddr != ""
	cfg.RedirectHTTPToHTTPS = httpsAddr != ""
	cfg.RateLimit.Enabled = false
	return SaveInfra(db, cfg)
}

// OverlayGithub aplica settings (origen setup browser) sobre el bloque
// github de la infra. La DB manda si tiene app_id.
func OverlayGithub(cfg *config.Config, db *store.DB) error {
	if v := db.GetSetting("github_app_id"); v != "" {
		var id int64
		fmt.Sscanf(v, "%d", &id)
		if id > 0 {
			if cfg.Github == nil {
				cfg.Github = &config.Github{}
			}
			cfg.Github.AppID = id
			cfg.Github.AppSlug = db.GetSetting("github_app_slug")
			cfg.Github.KeyFile = db.GetSetting("github_key_file")
			cfg.Github.WebhookSecret = db.GetSetting("github_webhook_secret")
			return cfg.Github.EnsureParsed()
		}
	}
	return nil
}

// SaveFleet valida (defaults + Validate) y persiste atómico.
// Camino único de escritura CLI/API (antes persist duplicado en app).
func SaveFleet(db *store.DB, cfg *config.Config) error {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	return db.ImportFleet(cfg)
}

// Snapshot fusiona infra (DB) + flota (DB).
func Snapshot(db *store.DB, base *config.Config) (*config.Config, error) {
	fleet, err := db.LoadFleet()
	if err != nil {
		return nil, err
	}
	out := *base
	out.Apps = fleet.Apps
	out.Routes = fleet.Routes
	out.ApplyDefaults()
	applyDashboard(&out, db)
	if err := config.ValidateRoutes(out.Routes); err != nil {
		return nil, fmt.Errorf("db %w", err)
	}
	return &out, nil
}

// applyDashboard resuelve la exposición de la consola desde settings.
// vacío/auto = rkt.<base_domain> si hay base; off = solo loopback.
func applyDashboard(cfg *config.Config, db *store.DB) {
	h := strings.ToLower(strings.TrimSpace(db.GetSetting(store.KeyDashboardHost)))
	switch h {
	case "off":
		cfg.Dashboard.Host = ""
	case "", "auto":
		base := strings.Trim(strings.TrimSpace(db.GetSetting(store.KeyBaseDomain)), ".")
		if base != "" {
			cfg.Dashboard.Host = "rkt." + base
		} else {
			cfg.Dashboard.Host = ""
		}
	default:
		cfg.Dashboard.Host = strings.Trim(h, ".")
	}
	cfg.Dashboard.Listen = strings.TrimSpace(db.GetSetting(store.KeyDashboardListen))
	if cfg.Dashboard.Listen == "off" {
		cfg.Dashboard.Listen = ""
	}
}
