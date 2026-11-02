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
