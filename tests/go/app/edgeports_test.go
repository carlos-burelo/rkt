package app

import (
	"testing"

	"rkt-proxy/internal/config"
)

// edgePorts: distintos y ordenados; sin rutas con puerto, vacío.
func TestEdgePorts(t *testing.T) {
	if got := edgePorts(&config.Config{}); len(got) != 0 {
		t.Fatalf("vacío = %v", got)
	}
	cfg := &config.Config{Routes: []config.Route{
		{Hosts: []string{"a.test"}, Target: "http://127.0.0.1:1"},
		{Port: 3001, Target: "http://127.0.0.1:2"},
		{Hosts: []string{"b.test"}, Port: 3000, Target: "http://127.0.0.1:3"},
		{Port: 3000, Target: "http://127.0.0.1:4"},
	}}
	got := edgePorts(cfg)
	if len(got) != 2 || got[0] != 3000 || got[1] != 3001 {
		t.Fatalf("distintos y ordenados = %v", got)
	}
}
