package doctor

import (
	"errors"
	"testing"
)

func testEnv() Env {
	return Env{
		LookPath: func(n string) (string, error) {
			switch n {
			case "git":
				return "/usr/bin/git", nil
			case "node":
				return "/usr/bin/node", nil
			default:
				return "", errors.New("no")
			}
		},
		Command: func(name string, args ...string) (string, error) {
			switch name {
			case "git":
				return "git version 2.43.0", nil
			case "node":
				return "v22.1.0", nil
			default:
				return "", errors.New("no responde")
			}
		},
		GetSetting: func(key string) string {
			if key == "port_pool" {
				return "3000-3002"
			}
			return ""
		},
		FreePort:   func(from, to int) (int, error) { return 3001, nil },
		EnsureDir:  func(string) error { return nil },
		WriteProbe: func(string) error { return nil },
	}
}

func find(cs []Check, name string) Check {
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	return Check{}
}

func TestRunBasico(t *testing.T) {
	cs := Run(testEnv())
	if got := find(cs, "git"); got.State != OK || got.Detail != "git version 2.43.0" {
		t.Fatalf("git = %+v", got)
	}
	if got := find(cs, "node"); got.State != OK {
		t.Fatalf("node = %+v", got)
	}
	if got := find(cs, "go"); got.State != Warn || got.Hint == "" {
		t.Fatalf("go ausente debe avisar con hint: %+v", got)
	}
	if got := find(cs, "node (settings)"); got.State != Warn {
		t.Fatalf("node_bin vacío debe avisar: %+v", got)
	}
	if got := find(cs, "port_pool"); got.State != OK {
		t.Fatalf("pool = %+v", got)
	}
	if Failed(cs) != 0 || Warned(cs) == 0 {
		t.Fatalf("failed=%d warned=%d", Failed(cs), Warned(cs))
	}
}

func TestGitFaltaEsBloqueante(t *testing.T) {
	e := testEnv()
	e.LookPath = func(string) (string, error) { return "", errors.New("no") }
	cs := Run(e)
	if got := find(cs, "git"); got.State != Fail {
		t.Fatalf("git = %+v", got)
	}
	if Failed(cs) == 0 {
		t.Fatal("sin git debe haber bloqueantes")
	}
}

func TestPoolInvalidoYPuertoAgotado(t *testing.T) {
	e := testEnv()
	e.GetSetting = func(key string) string {
		if key == "port_pool" {
			return "9999-1"
		}
		return ""
	}
	if got := find(Run(e), "port_pool"); got.State != Fail {
		t.Fatalf("pool inválido = %+v", got)
	}
	e.GetSetting = func(key string) string {
		if key == "port_pool" {
			return "3000-3001"
		}
		return ""
	}
	e.FreePort = func(int, int) (int, error) { return 0, errors.New("agotado") }
	if got := find(Run(e), "port_pool"); got.State != Fail || got.Hint == "" {
		t.Fatalf("pool agotado = %+v", got)
	}
}

func TestBinarioMalConfigurado(t *testing.T) {
	e := testEnv()
	e.GetSetting = func(key string) string {
		if key == "node_bin" {
			return "relativo/node"
		}
		if key == "python_bin" {
			return `C:\noexiste\python.exe`
		}
		return ""
	}
	cs := Run(e)
	if got := find(cs, "node (settings)"); got.State != Fail {
		t.Fatalf("relativo = %+v", got)
	}
	if got := find(cs, "python (settings)"); got.State != Fail {
		t.Fatalf("inexistente = %+v", got)
	}
}

func TestDirWritable(t *testing.T) {
	e := testEnv()
	if got := DirWritable(e, t.TempDir()); got.State != OK {
		t.Fatalf("dir temporal = %+v", got)
	}
	e.WriteProbe = func(string) error { return errors.New("ro") }
	if got := DirWritable(e, t.TempDir()); got.State != Fail {
		t.Fatalf("ro = %+v", got)
	}
}
