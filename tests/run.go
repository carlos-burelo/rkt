// Command run executes the repository's Go tests from their central source tree.
// Usage: go run ./tests/run.go test ./... -count=1
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "test" && os.Args[1] != "vet") {
		fatal("uso: go run ./tests/run.go test|vet [argumentos de go]")
	}
	root, err := os.Getwd()
	if err != nil {
		fatal("directorio actual: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fatal("ejecuta desde la raíz del repositorio: %v", err)
	}
	stored := filepath.Join(root, "tests", "go")
	replace := make(map[string]string)
	err = filepath.WalkDir(stored, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(stored, path)
		if err != nil {
			return err
		}
		replace[filepath.Join(root, "internal", rel)] = path
		return nil
	})
	if err != nil {
		fatal("leer pruebas Go: %v", err)
	}
	if len(replace) == 0 {
		fatal("no hay pruebas Go en tests/go")
	}
	config, err := os.CreateTemp("", "rkt-tests-overlay-*.json")
	if err != nil {
		fatal("crear overlay: %v", err)
	}
	defer os.Remove(config.Name())
	if err := json.NewEncoder(config).Encode(struct {
		Replace map[string]string
	}{replace}); err != nil {
		fatal("escribir overlay: %v", err)
	}
	if err := config.Close(); err != nil {
		fatal("cerrar overlay: %v", err)
	}
	args := append([]string{os.Args[1], "-overlay=" + config.Name()}, os.Args[2:]...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		fatal("ejecutar go: %v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
