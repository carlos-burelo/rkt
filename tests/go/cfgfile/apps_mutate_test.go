package cfgfile

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rkt/internal/config"
	"rkt/internal/discover"
	"rkt/internal/store"
)

// mkApp base válida para AddApp directo (puerto libre por defecto para no
// chocar entre tests; el caller lo pisa si necesita uno concreto).
func mkApp(t *testing.T, name string) config.App {
	t.Helper()
	dir, err := os.MkdirTemp("", "mkapp-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return config.App{
		Name: name, Command: `C:\x\rkt.exe`, Dir: dir,
		Env:          map[string]string{"PORT": "4699"},
		RestartDelay: "5s", MinUptime: "10s",
	}
}

func TestAddAppValidaTodo(t *testing.T) {
	newDB := func(t *testing.T) *store.DB {
		t.Helper()
		db, err := store.Open(filepath.Join(t.TempDir(), "rkt.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	t.Run("duplicado", func(t *testing.T) {
		db := newDB(t)
		a := mkApp(t, "dup")
		a.Env["PORT"] = "4610"
		if err := AddApp(db, a, []string{"dup.test"}, "4610"); err != nil {
			t.Fatal(err)
		}
		b := mkApp(t, "dup")
		b.Env["PORT"] = "4611"
		if err := AddApp(db, b, []string{"dup2.test"}, "4611"); err == nil || !strings.Contains(err.Error(), "ya existe") {
			t.Fatalf("duplicado: %v", err)
		}
	})
	t.Run("puerto duplicado", func(t *testing.T) {
		db := newDB(t)
		a := mkApp(t, "a")
		a.Env["PORT"] = "4610"
		if err := AddApp(db, a, []string{"a.test"}, "4610"); err != nil {
			t.Fatal(err)
		}
		b := mkApp(t, "b")
		b.Env["PORT"] = "4610"
		if err := AddApp(db, b, []string{"b.test"}, "4610"); err == nil || !strings.Contains(err.Error(), "ya usado") {
			t.Fatalf("puerto duplicado: %v", err)
		}
	})
	t.Run("nombre inválido", func(t *testing.T) {
		db := newDB(t)
		bad := mkApp(t, "Mala_Nombre")
		bad.Env["PORT"] = "4612"
		if err := AddApp(db, bad, []string{"mala.test"}, "4612"); err == nil || !strings.Contains(err.Error(), "name inválido") {
			t.Fatalf("nombre inválido: %v", err)
		}
	})
	t.Run("host vacío", func(t *testing.T) {
		db := newDB(t)
		if err := AddApp(db, mkApp(t, "h"), []string{"   "}, "4613"); err == nil {
			t.Fatal("host vacío debió fallar")
		}
	})
	t.Run("sin dominios no crea rutas", func(t *testing.T) {
		db := newDB(t)
		if err := AddApp(db, mkApp(t, "sola"), nil, ""); err != nil {
			t.Fatal(err)
		}
		fleet, err := db.LoadFleet()
		if err != nil {
			t.Fatal(err)
		}
		if len(fleet.Apps) != 1 || len(fleet.Routes) != 0 {
			t.Fatalf("apps=%d routes=%d", len(fleet.Apps), len(fleet.Routes))
		}
	})
	t.Run("defaults y target", func(t *testing.T) {
		db := newDB(t)
		a := mkApp(t, "dflt")
		a.RestartDelay, a.MinUptime, a.Runtime = "", "", ""
		a.Env["PORT"] = "4614"
		if err := AddApp(db, a, []string{"dflt.test"}, "4614"); err != nil {
			t.Fatal(err)
		}
		fleet, err := db.LoadFleet()
		if err != nil {
			t.Fatal(err)
		}
		got := fleet.Apps[0]
		if got.RestartDelay != "5s" || got.MinUptime != "10s" || got.Runtime != config.RuntimeManaged {
			t.Fatalf("defaults no aplicados: %+v", got)
		}
		if len(fleet.Routes) != 1 || fleet.Routes[0].Target != "http://127.0.0.1:4614" {
			t.Fatalf("ruta = %+v", fleet.Routes)
		}
		if got.Env["PORT"] != "4614" {
			t.Fatalf("Env PORT = %q", got.Env["PORT"])
		}
	})
	t.Run("atómico ante error", func(t *testing.T) {
		db := newDB(t)
		if err := AddApp(db, mkApp(t, "a"), []string{"a.test"}, "4615"); err != nil {
			t.Skip("puerto de fixture ocupado")
		}
		if err := AddApp(db, mkApp(t, "b"), []string{"no es host"}, "4616"); err == nil {
			t.Fatal("debió fallar")
		}
		fleet, err := db.LoadFleet()
		if err != nil {
			t.Fatal(err)
		}
		if len(fleet.Apps) != 1 || len(fleet.Routes) != 1 {
			t.Fatalf("fallo parcial: apps=%d routes=%d", len(fleet.Apps), len(fleet.Routes))
		}
	})
}

func TestUpdateAppDominiosYPuertos(t *testing.T) {
	mk := func() *config.Config {
		cfg := &config.Config{
			Apps: []config.App{{
				Name: "w", Command: `C:\x\rkt.exe`, Dir: t.TempDir(),
				Env: map[string]string{"PORT": "4720"}, RestartDelay: "5s", MinUptime: "10s",
			}},
			Routes: []config.Route{{Hosts: []string{"w.test"}, Target: "http://127.0.0.1:4720"}},
		}
		cfg.ApplyDefaults()
		return cfg
	}
	cp := mk()
	port := 4721
	if err := UpdateApp(cp, "w", AppPatch{Port: &port}); err != nil {
		t.Fatal(err)
	}
	if cp.Apps[0].Env["PORT"] != "4721" {
		t.Fatalf("PORT = %q", cp.Apps[0].Env["PORT"])
	}
	if len(cp.Routes) != 1 || cp.Routes[0].Target != "http://127.0.0.1:4721" {
		t.Fatalf("rutas sin retarget: %+v", cp.Routes)
	}
	doms := []string{"nuevo.test", "alias.test"}
	if err := UpdateApp(cp, "w", AppPatch{Domains: &doms}); err != nil {
		t.Fatal(err)
	}
	if len(cp.Routes) != 1 || len(cp.Routes[0].Hosts) != 2 || cp.Routes[0].Hosts[0] != "nuevo.test" {
		t.Fatalf("dominios sin reemplazar: %+v", cp.Routes)
	}
	dis := true
	if err := UpdateApp(cp, "w", AppPatch{Disabled: &dis}); err != nil {
		t.Fatal(err)
	}
	if !cp.Apps[0].Disabled {
		t.Fatal("disabled no aplicado")
	}
}

func TestUpdateAppInexistenteEInvalida(t *testing.T) {
	mk := func() *config.Config {
		cfg := &config.Config{
			Apps: []config.App{{
				Name: "w", Command: `C:\x\rkt.exe`, Dir: t.TempDir(),
				Env: map[string]string{"PORT": "4722"}, RestartDelay: "5s", MinUptime: "10s",
			}},
			Routes: []config.Route{{Hosts: []string{"w.test"}, Target: "http://127.0.0.1:4722"}},
		}
		cfg.ApplyDefaults()
		return cfg
	}
	cp := mk()
	snap := cp.Clone()
	if err := UpdateApp(cp, "fantasma", AppPatch{}); err == nil || !strings.Contains(err.Error(), "inexistente") {
		t.Fatalf("inexistente: %v", err)
	}
	bad := "no-slash"
	if err := UpdateApp(cp, "w", AppPatch{HealthPath: &bad}); err == nil {
		t.Fatal("health_path sin barra inicial debe fallar")
	}
	badPort := 99999
	if err := UpdateApp(cp, "w", AppPatch{Port: &badPort}); err == nil {
		t.Fatal("puerto 99999 debe fallar")
	}
	if cp.Apps[0].Env["PORT"] != snap.Apps[0].Env["PORT"] || len(cp.Routes) != len(snap.Routes) {
		t.Fatal("fallo parcial: la config quedo mutada")
	}
}

func TestRemoveAppRutasYOpcion(t *testing.T) {
	mk := func(name, port string) config.App {
		return config.App{Name: name, Command: `C:\x\rkt.exe`, Dir: t.TempDir(),
			Env: map[string]string{"PORT": port}, RestartDelay: "5s", MinUptime: "10s"}
	}
	newCfg := func() *config.Config {
		cp := &config.Config{}
		for _, a := range []config.App{mk("a", "4630"), mk("b", "4631")} {
			cp.Apps = append(cp.Apps, a)
		}
		cp.Routes = []config.Route{
			{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:4630"},
			{Hosts: []string{"b.test"}, Target: "http://127.0.0.1:4631"},
		}
		cp.ApplyDefaults()
		if err := cp.Validate(); err != nil {
			t.Fatalf("fixture inválida: %v", err)
		}
		return cp
	}
	cp := newCfg()
	n, err := RemoveApp(cp, "a", false)
	if err != nil || n != 1 {
		t.Fatalf("remove: %d %v", n, err)
	}
	for _, r := range cp.Routes {
		for _, h := range r.Hosts {
			if h == "a.test" {
				t.Fatalf("ruta huérfana: %+v", cp.Routes)
			}
		}
	}
	if _, err := RemoveApp(cp, "a", false); err == nil {
		t.Fatal("doble remove debió fallar")
	}
	n, err = RemoveApp(cp, "b", true)
	if err != nil || n != 0 {
		t.Fatalf("keepRoutes: %d %v", n, err)
	}
	if len(cp.Routes) != 1 {
		t.Fatalf("keepRoutes borró: %+v", cp.Routes)
	}
}

func TestFreePortPoolAgotadoYDefecto(t *testing.T) {
	db := newTestDB(t)
	if err := db.SetSetting(store.KeyPortPool, "35000-35001"); err != nil {
		t.Fatal(err)
	}
	l1, err := net.Listen("tcp", "127.0.0.1:35000")
	if err != nil {
		t.Skip("puerto de prueba ocupado")
	}
	defer l1.Close()
	l2, err := net.Listen("tcp", "127.0.0.1:35001")
	if err != nil {
		t.Skip("puerto de prueba ocupado")
	}
	defer l2.Close()
	if _, err := discover.FreePort(db); err == nil || !strings.Contains(err.Error(), "agotado") {
		t.Fatalf("pool agotado: %v", err)
	}
	if err := db.SetSetting(store.KeyPortPool, "no-es-un-rango"); err != nil {
		t.Fatal(err)
	}
	p, err := discover.FreePort(db)
	if err != nil {
		t.Fatalf("pool inválido debe caer al default: %v", err)
	}
	if p < 3000 || p > 3999 {
		t.Fatalf("puerto fuera del default: %d", p)
	}
}
