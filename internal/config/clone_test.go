package config

import "testing"

func TestCloneEsProfundo(t *testing.T) {
	src := &Config{
		GlobalEnv: map[string]string{"G": "1"},
		DenyPaths: []string{"/.env"},
		Github:    &Github{AppID: 7, AppSlug: "rkt"},
		Apps: []App{{
			Name: "w", Command: "cmd",
			Args:         []string{"a"},
			Env:          map[string]string{"K": "v"},
			RestartDelay: "5s", MinUptime: "10s",
			Deploy: &Deploy{Repo: "r", Ref: "main", Build: []string{"b"}},
		}},
		Routes: []Route{{Hosts: []string{"w.test"}, Target: "http://127.0.0.1:1"}},
	}
	cp := src.Clone()

	cp.GlobalEnv["G"] = "2"
	cp.DenyPaths[0] = "/x"
	cp.Github.AppID = 9
	cp.Apps[0].Args[0] = "b"
	cp.Apps[0].Env["K"] = "x"
	cp.Apps[0].Deploy.Build[0] = "y"
	cp.Routes[0].Hosts[0] = "z.test"

	if src.GlobalEnv["G"] != "1" || src.DenyPaths[0] != "/.env" || src.Github.AppID != 7 {
		t.Fatal("campos de config compartidos")
	}
	if src.Apps[0].Args[0] != "a" || src.Apps[0].Env["K"] != "v" ||
		src.Apps[0].Deploy.Build[0] != "b" || src.Routes[0].Hosts[0] != "w.test" {
		t.Fatal("campos de app/ruta compartidos")
	}
	if (*Config)(nil).Clone() != nil {
		t.Fatal("Clone(nil) debe ser nil")
	}
}
