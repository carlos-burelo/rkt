// Package cfgfile: configuración en SQLite (única fuente). El YAML solo
// se lee una vez para migrar instalaciones viejas y queda como .migrated.
// El CLI muta por aquí; el servicio solo lee.
package cfgfile

import (
	"encoding/json"
	"fmt"
	"os"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
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

// LoadBase infra desde DB. Si no hay fila, migra el YAML una vez o usa
// defaults y persiste. Deja el YAML como .migrated (nunca más se lee).
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
		if err := base.Finalize(); err != nil {
			return nil, err
		}
		return base, nil
	}

	base := config.Default()
	yamlPath := logx.Abs(cfgPath)
	if st, err := os.Stat(yamlPath); err == nil && !st.IsDir() {
		if y, err := config.Load(yamlPath); err == nil {
			base = y
			if db.ConfigVersion() == 0 {
				if err := db.ImportFleet(y); err != nil {
					return nil, err
				}
				if y.Github != nil {
					seedGithubSettings(db, y.Github)
				}
			}
			_ = os.Rename(yamlPath, yamlPath+".migrated")
		}
	}
	if err := base.Finalize(); err != nil {
		return nil, err
	}
	if err := SaveInfra(db, base); err != nil {
		return nil, err
	}
	return base, nil
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

// seedGithubSettings vuelca el bloque YAML a settings (migración una vez).
func seedGithubSettings(db *store.DB, gh *config.Github) {
	set := func(k, v string) {
		if v != "" {
			_ = db.SetSetting(k, v)
		}
	}
	set("github_app_id", itoaOrEmpty(gh.AppID))
	set("github_app_slug", gh.AppSlug)
	set("github_key_file", gh.KeyFile)
	set("github_webhook_secret", gh.WebhookSecret)
}

func itoaOrEmpty(n int64) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
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
	if err := config.ValidateRoutes(out.Routes); err != nil {
		return nil, fmt.Errorf("db %w", err)
	}
	return &out, nil
}
