package cfgfile

import (
	"context"
	"encoding/json"
	"fmt"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/store"
)

// MutateFleet ejecuta un read-modify-write de flota en una sola transacción
// inmediata: ningún otro proceso puede colarse entre la lectura y el guardado.
func MutateFleet(db *store.DB, fn func(*config.Config) error) (*config.Config, error) {
	var out *config.Config
	err := db.WithTx(context.Background(), func(tx *store.Tx) error {
		cfg, err := snapshotTx(tx)
		if err != nil {
			return err
		}
		if fn != nil {
			if err := fn(cfg); err != nil {
				return err
			}
		}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("inválido, no guardado: %w", err)
		}
		if err := tx.ImportFleet(cfg); err != nil {
			return err
		}
		out = cfg
		return nil
	})
	return out, err
}

// MutateInfra igual que MutateFleet pero persiste la parte de infraestructura
// (global_env, settings estructurales) y deja la flota intacta.
func MutateInfra(db *store.DB, fn func(*config.Config) error) (*config.Config, error) {
	var out *config.Config
	err := db.WithTx(context.Background(), func(tx *store.Tx) error {
		cfg, err := snapshotTx(tx)
		if err != nil {
			return err
		}
		if fn != nil {
			if err := fn(cfg); err != nil {
				return err
			}
		}
		cfg.ApplyDefaults()
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("inválido, no guardado: %w", err)
		}
		raw, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		if err := tx.SaveInfra(raw); err != nil {
			return err
		}
		out = cfg
		return nil
	})
	return out, err
}

// ReplaceAll reemplaza infra + flota + settings en una sola transacción.
func ReplaceAll(db *store.DB, cfg *config.Config, settings map[string]string) error {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("inválido, no guardado: %w", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return db.WithTx(context.Background(), func(tx *store.Tx) error {
		if err := tx.SaveInfra(raw); err != nil {
			return err
		}
		if err := tx.ImportFleet(cfg); err != nil {
			return err
		}
		for k, v := range settings {
			if err := tx.SetSetting(k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

// snapshotTx fusiona infra + flota dentro de la transacción.
func snapshotTx(tx *store.Tx) (*config.Config, error) {
	raw, err := tx.LoadInfra()
	if err != nil {
		return nil, err
	}
	base := config.Default()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, base); err != nil {
			return nil, fmt.Errorf("infra corrupta en DB: %w", err)
		}
	}
	fleet, err := tx.LoadFleet()
	if err != nil {
		return nil, err
	}
	cfg := *base
	cfg.Apps = fleet.Apps
	cfg.Routes = fleet.Routes
	cfg.ApplyDefaults()
	if err := config.ValidateRoutes(cfg.Routes); err != nil {
		return nil, fmt.Errorf("db %w", err)
	}
	return &cfg, nil
}
