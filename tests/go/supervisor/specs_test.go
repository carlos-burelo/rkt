package supervisor

import (
	"testing"

	"rkt-proxy/internal/config"
)

func TestSpecsEqualDetectaCambiosDeProceso(t *testing.T) {
	base := config.App{Name: "a", Command: "node", Args: []string{"server.js"}, Dir: "/x", Env: map[string]string{"PORT": "3000"}}
	if !specsEqual(base, base) {
		t.Fatal("misma spec debe ser igual")
	}
	cases := map[string]config.App{
		"command": {Name: "a", Command: "python", Args: []string{"server.js"}, Dir: "/x", Env: map[string]string{"PORT": "3000"}},
		"args":    {Name: "a", Command: "node", Args: []string{"index.js"}, Dir: "/x", Env: map[string]string{"PORT": "3000"}},
		"dir":     {Name: "a", Command: "node", Args: []string{"server.js"}, Dir: "/y", Env: map[string]string{"PORT": "3000"}},
		"env":     {Name: "a", Command: "node", Args: []string{"server.js"}, Dir: "/x", Env: map[string]string{"PORT": "3001"}},
	}
	for name, c := range cases {
		if specsEqual(base, c) {
			t.Fatalf("cambio de %s no detectado", name)
		}
	}
}

func TestSpecsEqualIgnoraMetadatosDeDeploy(t *testing.T) {
	a := config.App{Name: "a", Command: "node", Args: []string{"server.js"}, Dir: "/x"}
	b := a
	b.Deploy = &config.Deploy{Repo: "https://github.com/x/y", Ref: "main", Auto: true}
	if !specsEqual(a, b) {
		t.Fatal("deploy no debe forzar reinicio del proceso")
	}
}
