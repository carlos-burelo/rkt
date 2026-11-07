// Package settings valida los ajustes de flota. Store solo persiste.
package settings

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	KeyNodeBin    = "node_bin"
	KeyPythonBin  = "python_bin"
	KeyBaseDomain = "base_domain"
	KeyPortPool   = "port_pool"
	KeySiteName   = "site_name"
	KeyQuotaReqs  = "quota_reqs"
	KeyQuotaBytes = "quota_bytes"
	KeyPublicIP   = "public_ip"
	// Retención de historial y logs (días). 0 = conservar siempre.
	KeyRetentionDeployDays = "retention_deploy_days"
	KeyRetentionEventDays  = "retention_event_days"
	KeyRetentionLogDays    = "retention_log_days"
	KeyRetentionBackupDays = "retention_backup_days"
	// Consola pública: host en 80/443 y/o listener extra.
	KeyDashboardHost   = "dashboard_host"
	KeyDashboardListen = "dashboard_listen"
	// Listeners públicos (requieren reinicio del servicio).
	KeyListenHTTP  = "listen_http"
	KeyListenHTTPS = "listen_https"
)

// Spec describe un ajuste y si exige reiniciar el servicio.
type Spec struct {
	Key      string
	Restart  bool
	Validate func(string) error
}

var specs = []Spec{
	{Key: KeyNodeBin, Restart: false, Validate: validateExecutable},
	{Key: KeyPythonBin, Restart: false, Validate: validateExecutable},
	{Key: KeyBaseDomain, Restart: false, Validate: validateHost},
	{Key: KeyPortPool, Restart: false, Validate: validatePool},
	{Key: KeySiteName, Restart: false, Validate: validateName},
	{Key: KeyQuotaReqs, Restart: false, Validate: validateNonNegative},
	{Key: KeyQuotaBytes, Restart: false, Validate: validateNonNegative},
	{Key: KeyPublicIP, Restart: false, Validate: validateIP},
	{Key: KeyRetentionDeployDays, Restart: false, Validate: validateNonNegative},
	{Key: KeyRetentionEventDays, Restart: false, Validate: validateNonNegative},
	{Key: KeyRetentionLogDays, Restart: false, Validate: validateNonNegative},
	{Key: KeyRetentionBackupDays, Restart: false, Validate: validateNonNegative},
	{Key: KeyDashboardHost, Restart: false, Validate: validateDashboardHost},
	{Key: KeyDashboardListen, Restart: true, Validate: validateListenOrOff},
	{Key: KeyListenHTTP, Restart: true, Validate: validateListenAddr},
	{Key: KeyListenHTTPS, Restart: true, Validate: validateListenOrOff},
}

// Keys lista canónica ordenada.
func Keys() []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Key)
	}
	return out
}

// Get devuelve la spec de una clave.
func Get(key string) (Spec, bool) {
	for _, s := range specs {
		if s.Key == key {
			return s, true
		}
	}
	return Spec{}, false
}

// Validate comprueba valor para una clave conocida.
func Validate(key, value string) error {
	s, ok := Get(key)
	if !ok {
		return fmt.Errorf("setting desconocido: %s", key)
	}
	if strings.TrimSpace(value) == "" {
		return nil // vacío = valor por defecto
	}
	if err := s.Validate(value); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

// RequiresRestart indica si el cambio exige reiniciar el servicio.
func RequiresRestart(key string) bool {
	s, ok := Get(key)
	return ok && s.Restart
}

func validateExecutable(v string) error {
	if !filepath.IsAbs(v) {
		return fmt.Errorf("debe ser una ruta absoluta")
	}
	if st, err := os.Stat(v); err != nil || st.IsDir() {
		return fmt.Errorf("ruta inaccesible: %s", v)
	}
	return nil
}

func validateHost(v string) error {
	v = strings.Trim(strings.TrimSpace(v), ".")
	if v == "" {
		return nil
	}
	if strings.ContainsAny(v, " /\\") {
		return fmt.Errorf("host inválido")
	}
	if u, err := url.Parse("http://" + v); err != nil || u.Host != v {
		return fmt.Errorf("host inválido")
	}
	return nil
}

func validatePool(v string) error {
	parts := strings.Split(v, "-")
	if len(parts) == 1 {
		parts = strings.Split(v, ":")
	}
	if len(parts) != 2 {
		return fmt.Errorf("usa inicio-fin (ej 3000-3999)")
	}
	from, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	to, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || from < 1 || to > 65535 || from > to {
		return fmt.Errorf("rango inválido (1..65535, inicio ≤ fin)")
	}
	return nil
}

func validateName(v string) error {
	if len(v) > 80 {
		return fmt.Errorf("máximo 80 caracteres")
	}
	return nil
}

func validateNonNegative(v string) error {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n < 0 {
		return fmt.Errorf("entero ≥ 0")
	}
	return nil
}

// validateDashboardHost acepta "off", "auto" o un host sin esquema/ruta.
func validateDashboardHost(v string) error {
	if v == "off" || v == "auto" {
		return nil
	}
	if strings.Contains(v, "://") || strings.ContainsAny(v, " /\\\t\r\n") {
		return fmt.Errorf("host inválido (sin esquema ni ruta)")
	}
	return validateHost(v)
}

// validateListenAddr exige host:puerto para listeners explícitos.
func validateListenAddr(v string) error {
	if _, _, err := net.SplitHostPort(v); err != nil {
		return fmt.Errorf("usa host:puerto (ej 0.0.0.0:8090)")
	}
	return nil
}

// validateListenOrOff permite "off" para desactivar un listener (TLS).
func validateListenOrOff(v string) error {
	if v == "off" {
		return nil
	}
	return validateListenAddr(v)
}

func validateIP(v string) error {
	if net.ParseIP(strings.TrimSpace(v)) == nil {
		return fmt.Errorf("IP inválida")
	}
	return nil
}
