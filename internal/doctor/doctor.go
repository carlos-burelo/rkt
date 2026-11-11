// Package doctor: verifica el entorno del host para operar rkt
// (toolchains, binarios configurados, pool de puertos, DB escribible).
// Los chequeos fallidos son bloqueantes; los avisos, no.
package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// State de un chequeo: ok, warn (aviso) o fail (bloqueante).
type State string

const (
	OK   State = "ok"
	Warn State = "warn"
	Fail State = "fail"
)

// Check un resultado. Hint dice cómo arreglarlo (español, accionable).
type Check struct {
	Name   string `json:"name"`
	State  State  `json:"state"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// Env dependencias inyectables (tests usan fakes).
type Env struct {
	// LookPath resuelve un binario en PATH.
	LookPath func(string) (string, error)
	// Command ejecuta y devuelve la primera línea de salida combinada.
	Command func(name string, args ...string) (string, error)
	// GetSetting lee un setting (node_bin, python_bin, port_pool...).
	GetSetting func(key string) string
	// FreePort devuelve un puerto libre del pool.
	FreePort func(from, to int) (int, error)
	// EnsureDir crea el dir si falta (prueba de escritura).
	EnsureDir func(path string) error
	// WriteProbe escribe y borra un archivo (prueba de escritura real).
	WriteProbe func(dir string) error
}

// SystemEnv entorno real del proceso que ejecuta el doctor.
// Nota: el servicio corre como SYSTEM con otro PATH; si un binario se ve
// aquí pero el build falla en el servicio, fija node_bin/python_bin
// absolutos (settings set node_bin=C:\...).
func SystemEnv(getSetting func(string) string) Env {
	return Env{
		LookPath: exec.LookPath,
		Command: func(name string, args ...string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
			first := strings.TrimSpace(string(out))
			if i := strings.IndexByte(first, '\n'); i >= 0 {
				first = strings.TrimSpace(first[:i])
			}
			if len(first) > 80 {
				first = first[:80]
			}
			return first, err
		},
		GetSetting: getSetting,
		FreePort: func(from, to int) (int, error) {
			for p := from; p <= to; p++ {
				l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
				if err != nil {
					continue
				}
				_ = l.Close()
				return p, nil
			}
			return 0, fmt.Errorf("pool %d-%d agotado", from, to)
		},
		EnsureDir: func(path string) error { return os.MkdirAll(path, 0o755) },
		WriteProbe: func(dir string) error {
			f, err := os.CreateTemp(dir, ".doctor-*")
			if err != nil {
				return err
			}
			name := f.Name()
			_ = f.Close()
			return os.Remove(name)
		},
	}
}

// tool binario con versión rápida; versionArgs vacío = solo presencia.
type tool struct {
	name, bin string
	version   []string
	hint      string
}

var tools = []tool{
	{"git", "git", []string{"--version"}, "instala git (https://git-scm.com): sin git no hay deploys"},
	{"node", "node", []string{"--version"}, "instala nodejs LTS o fija settings set node_bin=C:\\...\\node.exe"},
	{"pnpm", "pnpm", []string{"--version"}, "instala pnpm (corepack enable) o usa otro gestor"},
	{"yarn", "yarn", nil, "opcional: solo si algún repo lo exige (detecta yarn.lock)"},
	{"bun", "bun", []string{"--version"}, "opcional: solo si algún repo lo exige (detecta bun.lock)"},
	{"python", "python", []string{"--version"}, "instala python o fija settings set python_bin=C:\\...\\python.exe"},
	{"go", "go", []string{"version"}, "instala go (https://go.dev) para apps con go.mod"},
	{"rust", "rustc", []string{"--version"}, "instala rustup (https://rustup.rs) para apps con Cargo.toml"},
	{"cargo", "cargo", []string{"--version"}, "rustup lo instala junto a rustc"},
	{"java", "java", []string{"-version"}, "instala un JDK 17+ para apps maven/gradle"},
	{"maven", "mvn", nil, "instala maven o usa wrapper ./mvnw (repos con pom.xml)"},
	{"gradle", "gradle", nil, "instala gradle o usa wrapper ./gradlew (repos con build.gradle)"},
	{"dotnet", "dotnet", []string{"--version"}, "instala .NET SDK para apps con .csproj"},
	{"php", "php", []string{"--version"}, "instala php para apps con composer.json"},
	{"composer", "composer", nil, "instala composer (https://getcomposer.org) para apps php"},
	{"ruby", "ruby", []string{"--version"}, "instala ruby para apps con Gemfile"},
	{"bundler", "bundle", nil, "gem install bundler para apps ruby"},
	{"deno", "deno", []string{"--version"}, "instala deno para apps con deno.json"},
	{"hugo", "hugo", nil, "instala hugo para sitios con hugo.toml"},
	{"mkdocs", "mkdocs", nil, "pip install mkdocs para sitios con mkdocs.yml"},
}

// Run ejecuta todos los chequeos con el entorno dado.
func Run(e Env) []Check {
	out := []Check{}
	// git es bloqueante: sin git no hay deploys.
	if path, err := e.LookPath("git"); err != nil {
		out = append(out, Check{Name: "git", State: Fail, Detail: "no instalado", Hint: tools[0].hint})
	} else if v, err := e.Command("git", "--version"); err != nil || v == "" {
		out = append(out, Check{Name: "git", State: Fail, Detail: path, Hint: "git no responde a --version"})
	} else {
		out = append(out, Check{Name: "git", State: OK, Detail: v})
	}
	// Toolchains: aviso si faltan (solo las apps de ese stack las necesitan).
	for _, tl := range tools[1:] {
		path, err := e.LookPath(tl.bin)
		if err != nil {
			out = append(out, Check{Name: tl.name, State: Warn, Detail: "no instalado", Hint: tl.hint})
			continue
		}
		if len(tl.version) == 0 {
			out = append(out, Check{Name: tl.name, State: OK, Detail: path})
			continue
		}
		if v, err := e.Command(tl.bin, tl.version...); err != nil || v == "" {
			out = append(out, Check{Name: tl.name, State: Warn, Detail: path, Hint: tl.hint})
			continue
		} else {
			out = append(out, Check{Name: tl.name, State: OK, Detail: v})
		}
	}
	// Binarios configurados: si están, deben existir y ser ejecutables.
	for _, tc := range []struct{ key, name, hint string }{
		{"node_bin", "node (settings)", "settings set node_bin=<ruta absoluta a node>"},
		{"python_bin", "python (settings)", "settings set python_bin=<ruta absoluta a python>"},
	} {
		v := ""
		if e.GetSetting != nil {
			v = strings.TrimSpace(e.GetSetting(tc.key))
		}
		if v == "" {
			out = append(out, Check{Name: tc.name, State: Warn, Detail: "sin fijar (usa PATH del servicio)", Hint: tc.hint + " (el servicio SYSTEM ve otro PATH que tu sesión)"})
			continue
		}
		if !filepath.IsAbs(v) {
			out = append(out, Check{Name: tc.name, State: Fail, Detail: v, Hint: "debe ser ruta absoluta: " + tc.hint})
			continue
		}
		if st, err := os.Stat(v); err != nil || st.IsDir() {
			out = append(out, Check{Name: tc.name, State: Fail, Detail: v, Hint: "ruta inaccesible: " + tc.hint})
			continue
		}
		out = append(out, Check{Name: tc.name, State: OK, Detail: v})
	}
	// Pool de puertos: formato válido y al menos un puerto libre.
	pool := ""
	if e.GetSetting != nil {
		pool = strings.TrimSpace(e.GetSetting("port_pool"))
	}
	if pool == "" {
		pool = "3000-3999"
	}
	from, to, err := parsePool(pool)
	if err != nil {
		out = append(out, Check{Name: "port_pool", State: Fail, Detail: pool, Hint: "settings set port_pool=3000-3999 (inicio ≤ fin, 1..65535)"})
	} else if e.FreePort == nil {
		out = append(out, Check{Name: "port_pool", State: OK, Detail: pool})
	} else if p, err := e.FreePort(from, to); err != nil {
		out = append(out, Check{Name: "port_pool", State: Fail, Detail: pool, Hint: "sin puertos libres: amplía el pool o libera puertos"})
	} else {
		out = append(out, Check{Name: "port_pool", State: OK, Detail: fmt.Sprintf("%s (libre %d)", pool, p)})
	}
	return out
}

// DirWritable verifica que el dir de logs/DB se puede crear y escribir.
func DirWritable(e Env, dir string) Check {
	if err := e.EnsureDir(dir); err != nil {
		return Check{Name: "logs_dir", State: Fail, Detail: dir, Hint: fmt.Sprintf("sin escritura en %s: revisa permisos", dir)}
	}
	if err := e.WriteProbe(dir); err != nil {
		return Check{Name: "logs_dir", State: Fail, Detail: dir, Hint: "directorio sin escritura: revisa permisos/ACL"}
	}
	return Check{Name: "logs_dir", State: OK, Detail: dir}
}

// Failed cuenta bloqueantes.
func Failed(cs []Check) int {
	n := 0
	for _, c := range cs {
		if c.State == Fail {
			n++
		}
	}
	return n
}

// Warned cuenta avisos.
func Warned(cs []Check) int {
	n := 0
	for _, c := range cs {
		if c.State == Warn {
			n++
		}
	}
	return n
}

func parsePool(s string) (int, int, error) {
	parts := strings.Split(s, "-")
	if len(parts) == 1 {
		parts = strings.Split(s, ":")
	}
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("formato")
	}
	from, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	to, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || from < 1 || to > 65535 || from > to {
		return 0, 0, fmt.Errorf("rango")
	}
	return from, to, nil
}
