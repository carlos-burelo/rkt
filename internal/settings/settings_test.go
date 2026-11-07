package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeysYGet(t *testing.T) {
	keys := Keys()
	if len(keys) != 14 || keys[0] != KeyNodeBin {
		t.Fatalf("keys = %v", keys)
	}
	if s, ok := Get(KeyBaseDomain); !ok || s.Key != KeyBaseDomain {
		t.Fatal("Get base_domain")
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("clave desconocida")
	}
}

func TestValidateSettings(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	ok := map[string]string{
		KeyNodeBin: exe, KeyPythonBin: exe, KeyBaseDomain: "ejemplo.com",
		KeyPortPool: "3000-3999", KeySiteName: "rkt", KeyQuotaReqs: "100",
		KeyQuotaBytes: "0", KeyPublicIP: "203.0.113.7",
		KeyRetentionDeployDays: "30", KeyRetentionEventDays: "30",
		KeyRetentionLogDays: "30", KeyRetentionBackupDays: "7",
		KeyDashboardHost: "rkt.ejemplo.com", KeyDashboardListen: "0.0.0.0:8090",
	}
	for k, v := range ok {
		if err := Validate(k, v); err != nil {
			t.Fatalf("%s=%q: %v", k, v, err)
		}
	}
	if err := Validate(KeyBaseDomain, "a b"); err == nil {
		t.Fatal("host inválido")
	}
	if err := Validate(KeyPortPool, "4000-3000"); err == nil {
		t.Fatal("rango invertido")
	}
	if err := Validate(KeyPortPool, "3000"); err == nil {
		t.Fatal("rango incompleto")
	}
	if err := Validate(KeyQuotaReqs, "-1"); err == nil {
		t.Fatal("cuota negativa")
	}
	if err := Validate(KeyPublicIP, "nope"); err == nil {
		t.Fatal("IP inválida")
	}
	if err := Validate(KeyNodeBin, "relativo"); err == nil {
		t.Fatal("ruta relativa")
	}
	if err := Validate(KeyNodeBin, filepath.Join(t.TempDir(), "no")); err == nil {
		t.Fatal("ruta inexistente")
	}
	if err := Validate(KeySiteName, string(make([]byte, 81))); err == nil {
		t.Fatal("nombre largo")
	}
	if err := Validate("nope", "x"); err == nil {
		t.Fatal("clave desconocida")
	}
	if err := Validate(KeySiteName, ""); err != nil {
		t.Fatalf("vacío debe permitir default: %v", err)
	}
}

func TestRequiresRestart(t *testing.T) {
	for _, k := range Keys() {
		want := k == KeyDashboardListen
		if RequiresRestart(k) != want {
			t.Fatalf("%s RequiresRestart = %v, quiere %v", k, RequiresRestart(k), want)
		}
	}
	if RequiresRestart("nope") {
		t.Fatal("clave desconocida")
	}
}
