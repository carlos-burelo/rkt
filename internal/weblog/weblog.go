// Package weblog: parser puro del access.log (formato combinado +
// extras host=/dur=) y agregados para usage/analytics/observability.
// Sin IO de red ni DB: testeable puro. Lee access.log y access.log.1.
package weblog

import (
	"bufio"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry línea parseada.
type Entry struct {
	IP      string
	Time    time.Time
	Method  string
	Path    string // sin query
	Status  int
	Bytes   int64
	Referer string
	UA      string
	Host    string
	Dur     time.Duration
}

// ParseLine una línea; ok=false si malformada (se salta, no falla).
func ParseLine(line string) (Entry, bool) {
	var e Entry
	// IP + resto
	sp := strings.IndexByte(line, ' ')
	if sp < 0 {
		return e, false
	}
	e.IP = line[:sp]
	rest := line[sp:]

	// [tiempo]
	ti0 := strings.IndexByte(rest, '[')
	ti1 := strings.IndexByte(rest, ']')
	if ti0 < 0 || ti1 < 0 || ti1 <= ti0 {
		return e, false
	}
	t, err := time.Parse("02/Jan/2006:15:04:05 -0700", rest[ti0+1:ti1])
	if err != nil {
		return e, false
	}
	e.Time = t
	rest = rest[ti1+1:]

	// "METHOD path PROTO"
	q := quoted(rest)
	if len(q) < 1 {
		return e, false
	}
	parts := strings.SplitN(q[0], " ", 3)
	if len(parts) != 3 {
		return e, false
	}
	e.Method = parts[0]
	if i := strings.IndexByte(parts[1], '?'); i >= 0 {
		e.Path = parts[1][:i]
	} else {
		e.Path = parts[1]
	}
	rest = afterQuoted(rest, 1)

	// status bytes
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return e, false
	}
	st, err := strconv.Atoi(fields[0])
	if err != nil {
		return e, false
	}
	e.Status = st
	if fields[1] != "-" {
		if b, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			e.Bytes = b
		}
	}
	rest = strings.Join(fields[2:], " ")

	// "referer" "ua" host= dur=
	qs := quoted(rest)
	if len(qs) >= 1 {
		e.Referer = qs[0]
	}
	if len(qs) >= 2 {
		e.UA = qs[1]
	}
	rest = afterQuoted(rest, 2)
	for _, f := range strings.Fields(rest) {
		if v, ok := strings.CutPrefix(f, "host="); ok {
			// el access log escribe host="..." (%q); se acepta con y sin comillas
			e.Host = strings.Trim(v, `"`)
		} else if v, ok := strings.CutPrefix(f, "dur="); ok {
			if d, err := time.ParseDuration(v); err == nil {
				e.Dur = d
			}
		}
	}
	if e.Path == "" {
		return e, false
	}
	return e, true
}

// quoted extrae los N primeros campos entre comillas (soporta \" simple).
func quoted(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		j := strings.IndexByte(s[i:], '"')
		if j < 0 {
			break
		}
		i += j + 1
		var b strings.Builder
		closed := false
		for i < len(s) {
			if s[i] == '"' {
				closed = true
				i++
				break
			}
			b.WriteByte(s[i])
			i++
		}
		if closed {
			out = append(out, b.String())
		}
	}
	return out
}

// afterQuoted devuelve el resto tras consumir n campos entre comillas.
func afterQuoted(s string, n int) string {
	for k := 0; k < n; k++ {
		j := strings.IndexByte(s, '"')
		if j < 0 {
			return ""
		}
		s = s[j+1:]
		j = strings.IndexByte(s, '"')
		if j < 0 {
			return ""
		}
		s = s[j+1:]
	}
	return s
}

// ParseFile lee access.log (+ .1 si existe), salta líneas malas.
func ParseFile(path string) []Entry {
	var out []Entry
	for _, p := range []string{path, path + ".1"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if e, ok := ParseLine(sc.Text()); ok {
				out = append(out, e)
			}
		}
		f.Close()
	}
	return out
}

// Summary agregado para usage/analytics/observability.
type Summary struct {
	Total   int            `json:"total"`
	Bytes   int64          `json:"bytes"`
	ByClass map[string]int `json:"by_class"` // 2xx/3xx/4xx/5xx
	ByDay   map[string]int `json:"by_day"`   // YYYY-MM-DD
	ByPath  []KV           `json:"by_path"`  // top rutas
	ByHost  []KV           `json:"by_host"`  // top hosts
	ByRef   []KV           `json:"by_ref"`   // top referers no vacíos
	P50     string         `json:"p50"`
	P95     string         `json:"p95"`
}

// KV par ordenado desc.
type KV struct {
	K string `json:"k"`
	V int    `json:"v"`
}

// Summarize agrega entradas (top=10 por defecto en cada ranking).
func Summarize(es []Entry) Summary {
	s := Summary{
		ByClass: map[string]int{},
		ByDay:   map[string]int{},
	}
	paths, hosts, refs := map[string]int{}, map[string]int{}, map[string]int{}
	var durs []int64
	for _, e := range es {
		s.Total++
		s.Bytes += e.Bytes
		s.ByClass[classOf(e.Status)]++
		s.ByDay[e.Time.Format("2006-01-02")]++
		paths[e.Path]++
		if e.Host != "" {
			hosts[e.Host]++
		}
		if e.Referer != "" && e.Referer != "-" {
			refs[e.Referer]++
		}
		if e.Dur > 0 {
			durs = append(durs, int64(e.Dur))
		}
	}
	s.ByPath = top(paths, 10)
	s.ByHost = top(hosts, 10)
	s.ByRef = top(refs, 5)
	if len(durs) > 0 {
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		s.P50 = time.Duration(durs[len(durs)/2]).String()
		s.P95 = time.Duration(durs[int(float64(len(durs))*0.95)]).String()
	}
	return s
}

func classOf(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500:
		return "5xx"
	default:
		return "otro"
	}
}

func top(m map[string]int, n int) []KV {
	out := make([]KV, 0, len(m))
	for k, v := range m {
		out = append(out, KV{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].V == out[j].V {
			return out[i].K < out[j].K
		}
		return out[i].V > out[j].V
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
