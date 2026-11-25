// Package geoip resuelve país por IP con la base DB-IP "country lite"
// (CSV start,end,país; CC BY 4.0, https://db-ip.com). Sin dependencias y
// sin API externa en el camino caliente: el archivo se descarga una vez
// (Update) y se consulta en memoria. Sin archivo, Country devuelve "".
package geoip

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type v4Range struct {
	lo, hi uint32
	cc     [2]byte
}

type v6Range struct {
	lo, hi [16]byte
	cc     [2]byte
}

// Locator base en memoria que se recarga sola si el archivo cambia.
type Locator struct {
	path string

	mu      sync.RWMutex
	v4      []v4Range
	v6      []v6Range
	mtime   time.Time
	checked time.Time
}

// Open crea el localizador para path (el archivo puede no existir aún).
func Open(path string) *Locator { return &Locator{path: path} }

// Path ruta del archivo CSV.
func (l *Locator) Path() string { return l.path }

// Ready true si hay una base cargada.
func (l *Locator) Ready() bool {
	l.reload()
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.v4)+len(l.v6) > 0
}

// reload relee el archivo si cambió; stat como mucho cada 30 s.
func (l *Locator) reload() {
	l.mu.RLock()
	fresh := time.Since(l.checked) < 30*time.Second
	l.mu.RUnlock()
	if fresh {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.checked = time.Now()
	st, err := os.Stat(l.path)
	if err != nil || st.ModTime().Equal(l.mtime) {
		return
	}
	f, err := os.Open(l.path)
	if err != nil {
		return
	}
	defer f.Close()
	v4, v6, err := parse(f)
	if err != nil {
		return
	}
	l.v4, l.v6, l.mtime = v4, v6, st.ModTime()
}

// parse lee el CSV de DB-IP; filas inválidas se saltan.
func parse(r io.Reader) ([]v4Range, []v6Range, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	var v4 []v4Range
	var v6 []v6Range
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if len(rec) < 3 || len(rec[2]) != 2 {
			continue
		}
		lo, e1 := netip.ParseAddr(rec[0])
		hi, e2 := netip.ParseAddr(rec[1])
		if e1 != nil || e2 != nil || lo.Is4() != hi.Is4() {
			continue
		}
		cc := [2]byte{rec[2][0], rec[2][1]}
		if lo.Is4() {
			a, b := lo.As4(), hi.As4()
			v4 = append(v4, v4Range{u32(a), u32(b), cc})
		} else {
			v6 = append(v6, v6Range{lo.As16(), hi.As16(), cc})
		}
	}
	sort.Slice(v4, func(i, j int) bool { return v4[i].lo < v4[j].lo })
	sort.Slice(v6, func(i, j int) bool { return bytes.Compare(v6[i].lo[:], v6[j].lo[:]) < 0 })
	return v4, v6, nil
}

func u32(a [4]byte) uint32 {
	return uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
}

// Country código ISO-3166 alpha-2 de ip, o "" si no hay base o no consta.
func (l *Locator) Country(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	l.reload()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if addr.Is4() {
		n := u32(addr.As4())
		i := sort.Search(len(l.v4), func(i int) bool { return l.v4[i].lo > n }) - 1
		if i >= 0 && n <= l.v4[i].hi {
			return string(l.v4[i].cc[:])
		}
		return ""
	}
	b := addr.As16()
	i := sort.Search(len(l.v6), func(i int) bool { return bytes.Compare(l.v6[i].lo[:], b[:]) > 0 }) - 1
	if i >= 0 && bytes.Compare(b[:], l.v6[i].hi[:]) <= 0 {
		return string(l.v6[i].cc[:])
	}
	return ""
}

// maxDownload tope defensivo del CSV descomprimido.
const maxDownload = 256 << 20

// Update descarga la base del mes (o la anterior) y la deja en path de
// forma atómica. Devuelve la URL usada.
func Update(ctx context.Context, path string, now time.Time) (string, error) {
	var last error
	for _, t := range []time.Time{now, now.AddDate(0, -1, 0)} {
		url := fmt.Sprintf("https://download.db-ip.com/free/dbip-country-lite-%s.csv.gz", t.UTC().Format("2006-01"))
		if err := fetch(ctx, url, path); err != nil {
			last = err
			continue
		}
		return url, nil
	}
	return "", last
}

func fetch(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geoip %s: http %d", url, resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("geoip gzip: %w", err)
	}
	defer gz.Close()
	return install(io.LimitReader(gz, maxDownload), path)
}

// install valida el CSV y lo publica con rename (nunca deja uno a medias).
func install(r io.Reader, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "geoip-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	v4, v6, err := parse(f)
	f.Close()
	if err != nil || len(v4)+len(v6) == 0 {
		return fmt.Errorf("geoip: CSV inválido o vacío")
	}
	return os.Rename(tmp.Name(), path)
}
