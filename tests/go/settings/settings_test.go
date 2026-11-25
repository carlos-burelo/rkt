package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeysYGet(t *testing.T) {
	keys := Keys()
	if len(keys) != 28 || keys[0] != KeyNodeBin {
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
		KeyListenHTTP: ":80", KeyListenHTTPS: ":443",
		KeyPgMode: "local", KeyPgBin: exe, KeyPgPort: "5432",
		KeyPgDatabase: "rkt", KeyPgUser: "postgres", KeyPgPass: "root",
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
	for _, tc := range []struct {
		k, v  string
		want  bool
		label string
	}{
		{KeyPgMode, "off", true, "pg_mode off"},
		{KeyPgMode, "local", true, "pg_mode local"},
		{KeyPgMode, "otro", false, "pg_mode libre"},
		{KeyPgMode, "", true, "pg_mode vacío = default"},
		{KeyPgPort, "5432", true, "puerto válido"},
		{KeyPgPort, "0", false, "puerto 0"},
		{KeyPgPort, "99999", false, "puerto enorme"},
		{KeyPgPort, "x", false, "puerto texto"},
		{KeyPgDatabase, "rkt", true, "base válida"},
		{KeyPgDatabase, "MiDB", false, "mayúsculas en base"},
		{KeyPgDatabase, strings.Repeat("a", 64), false, "base larga"},
		{KeyPgDatabase, "con espacio", false, "base con espacio"},
		{KeyPgDatabase, "", true, "base vacía = default"},
		{KeyPgUser, "postgres", true, "superusuario válido"},
		{KeyPgUser, "Root", false, "mayúsculas en usuario"},
		{KeyPgPass, "root", true, "password libre"},
		{KeyPgPass, "", true, "password vacío = default"},
		{KeyPgBin, exe, true, "pg_bin ejecutable"},
		{KeySMTPHost, "smtp.ejemplo.com", true, "smtp host"},
		{KeySMTPHost, "con espacio", false, "smtp host inválido"},
		{KeySMTPPort, "587", true, "smtp puerto"},
		{KeySMTPPort, "x", false, "smtp puerto texto"},
		{KeySMTPFrom, "rkt@ejemplo.com", true, "remitente"},
		{KeySMTPFrom, "no-es-correo", false, "remitente inválido"},
		{KeySMTPSecurity, "tls", true, "seguridad tls"},
		{KeySMTPSecurity, "ssl", false, "seguridad desconocida"},
	} {
		if err := Validate(tc.k, tc.v); (err == nil) != tc.want {
			t.Fatalf("%s (%s=%q): err=%v", tc.label, tc.k, tc.v, err)
		}
	}
}

func TestRequiresRestart(t *testing.T) {
	for _, k := range Keys() {
		want := k == KeyDashboardListen || k == KeyListenHTTP || k == KeyListenHTTPS
		if RequiresRestart(k) != want {
			t.Fatalf("%s RequiresRestart = %v, quiere %v", k, RequiresRestart(k), want)
		}
	}
	if RequiresRestart("nope") {
		t.Fatal("clave desconocida")
	}
}

func TestSMTPPassEsSecreto(t *testing.T) {
	if !IsSecret(KeySMTPPass) || !IsSecret(KeyPgPass) || IsSecret(KeySMTPUser) {
		t.Fatal("solo pg_pass y smtp_pass son secretos")
	}
}
