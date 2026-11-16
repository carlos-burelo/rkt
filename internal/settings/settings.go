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

	"rkt/internal/config"
	"rkt/internal/pg"
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
	// Postgres gestionado (una DB compartida, un esquema por app).
	// rkt aprovisiona sobre la instancia local (no la ejecuta): crea DB,
	// esquemas y roles con el superusuario y deja regla trust en pg_hba
	// para loopback, así las apps conectan sin password en la URL.
	KeyPgMode     = "pg_mode"     // off|local (default off)
	KeyPgBin      = "pg_bin"      // ruta absoluta a pg_ctl.exe (vacío = buscar en PATH)
	KeyPgPort     = "pg_port"     // default 5432
	KeyPgDatabase = "pg_database" // default rkt
	KeyPgUser     = "pg_user"     // superusuario local (default postgres)
	KeyPgPass     = "pg_pass"     // password del superusuario (solo loopback)
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
	{Key: KeyPgMode, Restart: false, Validate: validatePgMode},
	{Key: KeyPgBin, Restart: false, Validate: validateExecutable},
	{Key: KeyPgPort, Restart: false, Validate: validatePort},
	{Key: KeyPgDatabase, Restart: false, Validate: validatePGIdent},
	{Key: KeyPgUser, Restart: false, Validate: validatePGIdent},
	{Key: KeyPgPass, Restart: false, Validate: validateAny},
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

// validatePgMode solo admite off (desactivado) o local (rkt lo supervisa).
func validatePgMode(v string) error {
	if v == "off" || v == "local" {
		return nil
	}
	return fmt.Errorf("usa off o local")
}

// validatePort puerto TCP 1..65535.
func validatePort(v string) error {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("puerto inválido (1..65535)")
	}
	return nil
}

// validatePGIdent identificador Postgres sin comillas ([a-z_][a-z0-9_$]*,
// máx 63). Sin mayúsculas: Postgres las pliega a minúsculas sin comillar.
// Hogar único: pg.ValidIdent.
func validatePGIdent(v string) error {
	return pg.ValidIdent(v)
}

// validateAny acepta cualquier valor no vacío (secretos sin formato);
// el vacío significa "por defecto" y lo filtra Validate antes de llegar.
func validateAny(string) error { return nil }

// IsSecret true para valores que nunca salen en claro (p. ej. pg_pass).
func IsSecret(key string) bool {
	return key == KeyPgPass
}

// DisplayValue enmascara secretos no vacíos para API/UI/MCP.
func DisplayValue(key, value string) string {
	if IsSecret(key) && strings.TrimSpace(value) != "" {
		return config.SecretMask
	}
	return value
}

// AcceptValue dice si un valor entrante se persiste (mask = conservar).
func AcceptValue(key, value string) (string, bool) {
	if IsSecret(key) && value == config.SecretMask {
		return "", false
	}
	return value, true
}

func validateIP(v string) error {
	if net.ParseIP(strings.TrimSpace(v)) == nil {
		return fmt.Errorf("IP inválida")
	}
	return nil
}
