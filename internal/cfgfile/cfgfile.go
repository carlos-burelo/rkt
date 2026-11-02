// Package cfgfile: lee/escribe config.yaml con backup y validación.
// El CLI muta por aquí; el servicio solo lee.
package cfgfile

import (
	"fmt"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

// Load lee y valida el YAML (solo infra/healthz; sin DB).
// Para CLI de solo-lectura (status, restart...). La flota vive en DB.
func Load(path string) (*config.Config, error) {
	return config.Load(path)
}

// OverlayGithub aplica settings (origen setup browser) sobre el bloque
// github del YAML. La DB manda si tiene app_id; si no, queda el YAML.
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

// LoadMerged abre DB, importa el YAML si la flota está fresca y fusiona
// infra YAML + flota DB. Devuelve config lista y DB abierta (cerrar).
func LoadMerged(cfgPath string) (*config.Config, *store.DB, error) {
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		return nil, nil, err
	}
	base, err := config.Load(cfgPath)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	if db.ConfigVersion() == 0 {
		if err := db.ImportFleet(base); err != nil {
			db.Close()
			return nil, nil, err
		}
		if base.Github != nil {
			seedGithubSettings(db, base.Github)
		}
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

// seedGithubSettings vuelca el bloque YAML a settings (una vez).
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

// Snapshot fusiona infra del YAML base + flota de DB.
// La DB solo guarda validado; aquí solo re-parseo mecánico (targets,
// duplicados). El env NO se re-fusiona (DB guarda ya expandido).
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
