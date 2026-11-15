package supervisor

// RebaseStatic: el estático absoluto del slot live se remapea al slot que
// sirve; fuera del live (externo) o vacío se deja tal cual.

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"rkt/internal/config"
)

func TestRebaseStaticTabla(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "web")
	serve := filepath.Join(root, "web.b")
	casos := []struct {
		name, static, want string
		ok                 bool
	}{
		{"dist del live va al que sirve", filepath.Join(live, "dist"), filepath.Join(serve, "dist"), true},
		{"slot raiz va al slot raiz", live, serve, true},
		{"externo no se toca", filepath.Join(root, "suelto"), filepath.Join(root, "suelto"), false},
		{"relativo no se toca", filepath.Join("dist"), filepath.Join("dist"), false},
	}
	for _, c := range casos {
		got, ok := RebaseStatic(live, c.static, serve)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q,%v), quiere (%q,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
	if _, ok := RebaseStatic("", filepath.Join(live, "dist"), serve); ok {
		t.Error("sin liveDir debe fallar")
	}
	if _, ok := RebaseStatic(live, filepath.Join(live, "dist"), ""); ok {
		t.Error("sin serveDir debe fallar")
	}
	if _, ok := RebaseStatic(live, "", serve); ok {
		t.Error("estatico vacio debe fallar")
	}
	// Hermano con prefijo común no es hijo: .. no sale del live.
	trampa := live + "2"
	if got, ok := RebaseStatic(live, filepath.Join(trampa, "dist"), serve); ok || got != filepath.Join(trampa, "dist") {
		t.Errorf("prefijo parecido no es hijo: got (%q,%v)", got, ok)
	}
}

// TestStartCanaryRemapeaEstatico: el canario sirve el slot inactivo aunque
// el __static guardado apunte al live (primer deploy: el live ni existe).
func TestStartCanaryRemapeaEstatico(t *testing.T) {
	live := filepath.Join(t.TempDir(), "web")
	inactive := live + ".b"
	want := filepath.Join(inactive, "dist")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	bin, _ := shell(t, "exit 0")
	apps := []config.App{{
		Name: "web", Command: bin,
		Args:   []string{"__static", filepath.Join(live, "dist")},
		Static: filepath.Join(live, "dist"), Dir: live,
	}}
	m := New(apps, nil, t.TempDir(), testLog(t))
	c, err := m.StartCanary("web", inactive, "1", io.Discard)
	if err != nil {
		t.Fatalf("canario: %v", err)
	}
	defer func() { _ = c.Wait() }()
	found := false
	for _, a := range c.Args {
		if a == want {
			found = true
		}
		if a == filepath.Join(live, "dist") {
			t.Fatalf("canario con path del live: %q", c.Args)
		}
	}
	if !found {
		t.Fatalf("canario sin path del inactivo %q: %q", want, c.Args)
	}
}
