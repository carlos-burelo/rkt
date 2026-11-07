// Package dotenv: parser mínimo de archivos .env (una línea por var).
// Formato: KEY=valor, # comentario, prefijo export opcional, comillas
// simples/dobles peladas. Sin escapes ni multilínea (literal).
package dotenv

import (
	"fmt"
	"os"
	"strings"
)

// ParseFile lee y parsea un .env. Archivo inexistente = error.
func ParseFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("leer env_file %s: %w", path, err)
	}
	return Parse(string(data))
}

// Parse convierte contenido .env a mapa. Línea malformada = error.
func Parse(content string) (map[string]string, error) {
	out := map[string]string{}
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.ReplaceAll(raw, "\r", ""))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export"))
		if line == "" {
			continue
		}
		key, val, hasEq := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !validKey(key) {
			return nil, fmt.Errorf("línea %d malformada: %q", i+1, raw)
		}
		if !hasEq {
			out[key] = ""
			continue
		}
		out[key] = unquote(strings.TrimSpace(val))
	}
	return out, nil
}

func validKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' ||
			(i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func unquote(s string) string {
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// EnvironMap foto del entorno del proceso como mapa.
func EnvironMap() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}

// EnvironList entorno + overrides (listo para exec.Cmd.Env).
func EnvironList(over map[string]string) []string {
	m := EnvironMap()
	for k, v := range over {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// inheritedOS variables del sistema que sí pasan a apps/builds. El resto
// (tokens, credenciales cloud, rutas de secretos) NO se hereda: un repo
// malicioso o una app no debe leer el entorno del servicio SYSTEM.
var inheritedOS = map[string]bool{
	"PATH": true, "PATHEXT": true, "COMSPEC": true, "SYSTEMROOT": true,
	"WINDIR": true, "SYSTEMDRIVE": true, "TEMP": true, "TMP": true,
	"TMPDIR": true, "HOME": true, "USERPROFILE": true, "APPDATA": true,
	"LOCALAPPDATA": true, "PROGRAMDATA": true, "PROGRAMFILES": true,
	"PROGRAMFILES(X86)": true, "OS": true, "NUMBER_OF_PROCESSORS": true,
	"PROCESSOR_ARCHITECTURE": true, "LANG": true, "LC_ALL": true, "TZ": true,
	"SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	"http_proxy": true, "https_proxy": true, "no_proxy": true,
	"GOROOT": true, "GOPATH": true, "GOMODCACHE": true, "GOCACHE": true,
	"CARGO_HOME": true, "RUSTUP_HOME": true, "JAVA_HOME": true,
	"DOTNET_ROOT": true, "PNPM_HOME": true, "NPM_CONFIG_PREFIX": true,
	"PYTHONPATH": true, "VIRTUAL_ENV": true,
}

// EnvironListApp entorno saneado para procesos de app/build/cron:
// allowlist del OS + variables propias. Nunca hereda secretos de máquina.
func EnvironListApp(over map[string]string) []string {
	m := map[string]string{}
	for k, v := range EnvironMap() {
		if inheritedOS[strings.ToUpper(k)] {
			m[k] = v
		}
	}
	for k, v := range over {
		m[k] = v
	}
	// pnpm 12 re-instala antes de `run` si cree que node_modules está
	// desincronizado. En un PaaS eso ejecuta un install mientras la app
	// está viva y rompe node_modules en Windows. El build es el único
	// camino de instalación.
	if _, ok := m["npm_config_verify_deps_before_run"]; !ok {
		m["npm_config_verify_deps_before_run"] = "false"
	}
	if _, ok := m["COREPACK_ENABLE_DOWNLOAD_PROMPT"]; !ok {
		m["COREPACK_ENABLE_DOWNLOAD_PROMPT"] = "0"
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// Expand resuelve ${VAR} y $VAR en cada valor contra scope.
// scope previo = OS ∪ file; inline se aplica ordenado (determinista):
// cada clave ve OS+file+inline-anteriores. $ suelto se deja literal.
// Indefinida expande a "" (estilo dotenv).
func Expand(vars map[string]string, scope map[string]string) map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sortStrings(keys)
	base := map[string]string{}
	for k, v := range scope {
		base[k] = v
	}
	for _, k := range keys {
		out[k] = expandOne(vars[k], base)
		base[k] = out[k]
	}
	return out
}

func expandOne(s string, scope map[string]string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		if s[i+1] == '$' {
			b.WriteByte('$') // $$ = $ literal
			i += 2
			continue
		}
		if s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				b.WriteByte(s[i])
				i++
				continue
			}
			b.WriteString(scope[s[i+2:i+2+end]])
			i += 3 + end
			continue
		}
		j := i + 1
		for j < len(s) && isVarChar(s[j], j == i+1) {
			j++
		}
		if j == i+1 {
			b.WriteByte(s[i]) // $ suelto
			i++
			continue
		}
		b.WriteString(scope[s[i+1:j]])
		i = j
	}
	return b.String()
}

func isVarChar(c byte, first bool) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' ||
		(!first && c >= '0' && c <= '9')
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
