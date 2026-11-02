// Package cfgfile: lee/escribe config.yaml con backup y validación.
// El CLI muta por aquí; el servicio solo lee.
package cfgfile

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/logx"
	"rkt-proxy/internal/store"
)

// Load lee y valida el YAML (solo infra/healthz; sin DB).
// Para CLI de solo-lectura (status, restart...). La flota vive en DB.
func Load(path string) (*config.Config, error) {
	return config.Load(path)
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
	}
	merged, err := Snapshot(db, base)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return merged, db, nil
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
	seen := map[string]string{}
	for i := range out.Routes {
		r := &out.Routes[i]
		if len(r.Hosts) == 0 {
			return nil, fmt.Errorf("db route %d sin hosts", i)
		}
		if err := r.Resolve(); err != nil {
			return nil, fmt.Errorf("db route %d %w", i, err)
		}
		for _, h := range r.Hosts {
			h = strings.ToLower(strings.TrimSpace(h))
			if h == "" {
				return nil, fmt.Errorf("db route %d con host vacío", i)
			}
			if prev, ok := seen[h]; ok {
				return nil, fmt.Errorf("db host %q duplicado (en %q y %q)", h, prev, r.Target)
			}
			seen[h] = r.Target
		}
	}
	return &out, nil
}

// Save valida, respalda (config.yaml.bak-<ts>) y escribe.
// OJO: re-marshal pierde comentarios; el backup los conserva.
func Save(path string, cfg *config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	// valida lo que se va a escribir (round-trip)
	tmp, err := os.CreateTemp("", "rkt-cfg-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if _, err := config.Load(tmp.Name()); err != nil {
		return fmt.Errorf("config resultante inválida: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		bak := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
		if err := copyFile(path, bak); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
