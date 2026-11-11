package cli

import (
	"os"
	"strings"
	"testing"
)

func TestGithubInitURL(t *testing.T) {
	got := githubInitURL("app1.coatl.in", "rkt-deployer", "dacyti-gsi", "tok123")
	want := "https://app1.coatl.in/_rkt/new-app?name=rkt-deployer&domain=app1.coatl.in&org=dacyti-gsi&state=tok123"
	if got != want {
		t.Fatalf("got %q quiere %q", got, want)
	}
	got = githubInitURL("vps.test", "mi app", "", "")
	if !strings.HasPrefix(got, "https://vps.test/_rkt/new-app?name=mi+app&domain=vps.test") || strings.Contains(got, "org=") || strings.Contains(got, "state=") {
		t.Fatalf("escape/org/state mal: %q", got)
	}
}

func TestWantJSON(t *testing.T) {
	rest, asJSON := wantJSON([]string{"list", "--json"})
	if !asJSON || len(rest) != 1 || rest[0] != "list" {
		t.Fatalf("got %v %v", rest, asJSON)
	}
	rest, asJSON = wantJSON([]string{"--app", "x"})
	if asJSON || len(rest) != 2 {
		t.Fatalf("sin flag: %v %v", rest, asJSON)
	}
}

func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
}

func TestPrompt(t *testing.T) {
	withStdin(t, "\n")
	if got := prompt("nombre", "sugerido"); got != "sugerido" {
		t.Fatalf("enter: got %q", got)
	}
	withStdin(t, "  Mi Respuesta  \n")
	if got := prompt("nombre", "sugerido"); got != "Mi Respuesta" {
		t.Fatalf("texto: got %q", got)
	}
	// stdin cerrado = default (modo no interactivo)
	r, w, _ := os.Pipe()
	w.Close()
	r.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	if got := prompt("x", "d"); got != "d" {
		t.Fatalf("cerrado: got %q", got)
	}
}

func TestPromptBool(t *testing.T) {
	withStdin(t, "\n")
	if promptBool("auto", false) {
		t.Fatal("default n")
	}
	withStdin(t, "s\n")
	if !promptBool("auto", false) {
		t.Fatal("s = true")
	}
	withStdin(t, "n\n")
	if promptBool("auto", true) {
		t.Fatal("n gana al default")
	}
}
