package settings

import (
	"testing"

	"rkt/internal/config"
)

func TestDashboardTLSValores(t *testing.T) {
	for _, ok := range []string{"off", "auto", "AUTO", " auto "} {
		if err := Validate(KeyDashboardTLS, ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if err := Validate(KeyDashboardTLS, "on"); err == nil {
		t.Error("on no es un valor válido")
	}
	if !RequiresRestart(KeyDashboardTLS) {
		t.Error("cambiar el TLS del listener exige reinicio")
	}
}

func TestSecretosSeEnmascaranYSeConservan(t *testing.T) {
	if DisplayValue(KeySMTPPass, "x") != config.SecretMask || DisplayValue(KeySMTPPass, "") != "" || DisplayValue(KeySMTPUser, "u") != "u" {
		t.Fatal("DisplayValue")
	}
	if _, ok := AcceptValue(KeySMTPPass, config.SecretMask); ok {
		t.Fatal("la máscara no debe persistirse")
	}
	if v, ok := AcceptValue(KeySMTPPass, "nueva"); !ok || v != "nueva" {
		t.Fatal("un valor nuevo sí se acepta")
	}
	if v, ok := AcceptValue(KeySMTPUser, config.SecretMask); !ok || v != config.SecretMask {
		t.Fatal("solo los secretos usan la máscara")
	}
}
