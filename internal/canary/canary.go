// Package canary guarda binarios de rkt subidos sin pasar por git/release.
// Solo SUBIDA y custodia: aplicar un canary es un paso aparte (Launch, que
// delega en `rkt upgrade --local` con su gate de salud y rollback).
//
// Layout: <home>/canary/<id>/rkt.exe + meta.json. Hoja: solo stdlib.
package canary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Límites defensivos.
const (
	MaxSize = 256 << 20 // un rkt.exe pesa ~60 MB
	Keep    = 3         // canaries conservados (los más nuevos)
)

var (
	idRe    = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{8}$`)
	labelRe = regexp.MustCompile(`^[A-Za-z0-9._+-]{0,64}$`)
)

// Meta descripción de un canary subido.
type Meta struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty"` // lo que declara quien sube (ej: git describe)
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	UploadedAt string `json:"uploaded_at"`
	By         string `json:"by,omitempty"`
}

// Store directorio de canaries.
type Store struct{ dir string }

// Open abre el almacén bajo <home>/canary (se crea al subir).
func Open(home string) *Store { return &Store{dir: filepath.Join(home, "canary")} }

// ValidID true si id tiene la forma que genera Put (anti path traversal).
func ValidID(id string) bool { return idRe.MatchString(id) }

// Put guarda el binario: valida tamaño, formato ejecutable del SO y, si se
// da, el sha256 esperado. Escribe a temporal y publica con rename.
func (s *Store) Put(r io.Reader, label, wantSHA, by string, now time.Time) (Meta, error) {
	if !labelRe.MatchString(label) {
		return Meta{}, fmt.Errorf("label inválido: solo letras, dígitos y ._+- (máx 64)")
	}
	wantSHA = strings.ToLower(strings.TrimSpace(wantSHA))
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Meta{}, err
	}
	tmp, err := os.CreateTemp(s.dir, "upload-*.tmp")
	if err != nil {
		return Meta{}, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, MaxSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Meta{}, fmt.Errorf("subida: %w", err)
	}
	if n > MaxSize {
		return Meta{}, fmt.Errorf("binario mayor que %d MB", MaxSize>>20)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if wantSHA != "" && wantSHA != sum {
		return Meta{}, fmt.Errorf("sha256 no coincide: esperado %s, recibido %s", wantSHA, sum)
	}
	if err := checkExecutable(tmp.Name()); err != nil {
		return Meta{}, err
	}
	m := Meta{
		ID:         now.UTC().Format("20060102-150405") + "-" + sum[:8],
		Label:      label,
		SHA256:     sum,
		Size:       n,
		UploadedAt: now.UTC().Format(time.RFC3339),
		By:         by,
	}
	dst := filepath.Join(s.dir, m.ID)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return Meta{}, err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dst, "rkt.exe")); err != nil {
		return Meta{}, err
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dst, "meta.json"), raw, 0o644); err != nil {
		return Meta{}, err
	}
	s.prune()
	return m, nil
}

// checkExecutable exige la firma de ejecutable del SO donde corre rkt.
func checkExecutable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var head [4]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return fmt.Errorf("archivo demasiado corto para ser un ejecutable")
	}
	var ok bool
	switch runtime.GOOS {
	case "windows":
		ok = head[0] == 'M' && head[1] == 'Z'
	case "linux":
		ok = head == [4]byte{0x7f, 'E', 'L', 'F'}
	default: // mach-o (64 bits, ambos órdenes) o universal
		ok = head == [4]byte{0xcf, 0xfa, 0xed, 0xfe} || head == [4]byte{0xca, 0xfe, 0xba, 0xbe}
	}
	if !ok {
		return fmt.Errorf("no es un ejecutable de %s", runtime.GOOS)
	}
	return nil
}

// List canaries, el más nuevo primero.
func (s *Store) List() []Meta {
	ents, _ := os.ReadDir(s.dir)
	out := []Meta{}
	for _, e := range ents {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.dir, e.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(raw, &m) == nil && m.ID == e.Name() {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// Get devuelve meta y ruta del exe de un canary.
func (s *Store) Get(id string) (Meta, string, error) {
	if !ValidID(id) {
		return Meta{}, "", fmt.Errorf("id de canary inválido: %q", id)
	}
	for _, m := range s.List() {
		if m.ID == id {
			return m, filepath.Join(s.dir, id, "rkt.exe"), nil
		}
	}
	return Meta{}, "", fmt.Errorf("canary %s inexistente", id)
}

// Remove borra un canary.
func (s *Store) Remove(id string) error {
	if _, _, err := s.Get(id); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.dir, id))
}

// prune conserva los Keep más nuevos.
func (s *Store) prune() {
	list := s.List()
	for i := Keep; i < len(list); i++ {
		_ = os.RemoveAll(filepath.Join(s.dir, list[i].ID))
	}
}
