package deploy

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHURL(t *testing.T) {
	ok := map[string]string{
		"https://github.com/acme/web":      "git@github.com:acme/web.git",
		"https://github.com/acme/web.git":  "git@github.com:acme/web.git",
		"https://github.com/acme/web.git/": "git@github.com:acme/web.git",
		"http://git.local:8080/o/r":        "git@git.local:o/r.git", // el puerto http no es el de ssh
	}
	for in, want := range ok {
		if got, good := SSHURL(in); !good || got != want {
			t.Errorf("%s = %q,%v quiero %q", in, got, good, want)
		}
	}
	for _, bad := range []string{"git@github.com:acme/web.git", "https://github.com/solo", "https://github.com/a/b/c", "", "ftp://x/o/r"} {
		if _, good := SSHURL(bad); good {
			t.Errorf("%q no debería convertirse", bad)
		}
	}
}

// Con deploy key SSH en el contexto, git usa SOLO esa llave y sin prompts.
func TestGitCmdSSHEnv(t *testing.T) {
	key := filepath.Join(t.TempDir(), "k1.key")
	cmd := gitCmd(WithSSHKey(t.Context(), key), "", "ls-remote", "git@h:o/r.git")
	var ssh string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			ssh = e
		}
	}
	for _, want := range []string{"-i ", "IdentitiesOnly=yes", "BatchMode=yes", "StrictHostKeyChecking=accept-new", filepath.ToSlash(key)} {
		if !strings.Contains(ssh, want) {
			t.Fatalf("GIT_SSH_COMMAND sin %q: %q", want, ssh)
		}
	}
	for _, a := range cmd.Args {
		if strings.Contains(a, key) {
			t.Fatalf("la ruta de la llave no debe ir en argv: %v", cmd.Args)
		}
	}
	// sin llave no se toca GIT_SSH_COMMAND
	for _, e := range gitCmd(t.Context(), "", "ls-remote", "x").Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") && strings.Contains(e, "IdentitiesOnly") {
			t.Fatalf("sin llave no debe forzar identidad: %q", e)
		}
	}
}

// CheckAccess contra un repo real: alcanza uno existente, falla uno ausente.
func TestCheckAccess(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("sin git")
	}
	bare := filepath.Join(t.TempDir(), "r.git")
	work := t.TempDir()
	for _, c := range [][]string{
		{"init", "-q", "--bare", bare},
		{"-C", work, "init", "-q"},
		{"-C", work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"},
		{"-C", work, "push", "-q", bare, "HEAD:refs/heads/main"},
	} {
		if out, err := exec.Command("git", c...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", c, err, out)
		}
	}
	if err := CheckAccess(t.Context(), bare, ""); err != nil {
		t.Fatalf("repo existente: %v", err)
	}
	if err := CheckAccess(t.Context(), filepath.Join(t.TempDir(), "no-existe.git"), ""); err == nil {
		t.Fatal("repo ausente debe fallar")
	}
}
