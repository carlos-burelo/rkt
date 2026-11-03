package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirOrdenaCarpetasPrimero(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "zeta.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "alfa"), 0o755); err != nil {
		t.Fatal(err)
	}

	l, err := listDir(root)
	if err != nil {
		t.Fatalf("listDir: %v", err)
	}
	if l.Path != root {
		t.Fatalf("path = %q, quiero %q", l.Path, root)
	}
	if len(l.Entries) != 2 {
		t.Fatalf("entries = %d, quiero 2", len(l.Entries))
	}
	if !l.Entries[0].Dir || l.Entries[0].Name != "alfa" {
		t.Fatalf("primera entrada = %+v, quiero carpeta alfa", l.Entries[0])
	}
	if l.Entries[1].Dir || l.Entries[1].Name != "zeta.txt" {
		t.Fatalf("segunda entrada = %+v, quiero archivo zeta.txt", l.Entries[1])
	}
}

func TestListDirRutaInexistente(t *testing.T) {
	if _, err := listDir(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Fatal("quiero error para ruta inexistente")
	}
}

func TestListDirArchivoDevuelveSuCarpeta(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "info.log")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := listDir(file)
	if err != nil {
		t.Fatalf("listDir archivo: %v", err)
	}
	if l.Path != root {
		t.Fatalf("path = %q, quiero la carpeta %q", l.Path, root)
	}
}

func TestRootsListingNoVacio(t *testing.T) {
	if len(rootsListing().Entries) == 0 {
		t.Fatal("sin raíces de disco")
	}
}
