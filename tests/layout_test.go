package main

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Evita que una prueba nueva quede fuera del directorio central.
func TestSourcesStayInTests(t *testing.T) {
	for _, dir := range []string{"internal", filepath.Join("web", "src")} {
		err := filepath.WalkDir(filepath.Join("..", dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".test.tsx") {
				t.Errorf("prueba fuera de tests/: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
