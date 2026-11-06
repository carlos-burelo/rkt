package alerts

import (
	"testing"
	"time"

	"rkt-proxy/internal/store"
)

func TestFired(t *testing.T) {
	now := time.Now().UTC()
	r := store.AlertRule{ID: 1, Kind: "failed", Threshold: 2, WindowMin: 60, Enabled: true}
	if Fired(r, Metrics{Failed: 1}, now) {
		t.Fatal("1 < 2 no dispara")
	}
	if !Fired(r, Metrics{Failed: 2}, now) {
		t.Fatal("2 >= 2 dispara")
	}
	if Fired(store.AlertRule{Kind: "failed", Threshold: 1}, Metrics{Failed: 9}, now) {
		t.Fatal("regla desactivada no dispara")
	}
	r2 := store.AlertRule{ID: 2, Kind: "error_rate", Threshold: 0.1, WindowMin: 60, Enabled: true}
	if Fired(r2, Metrics{ErrRate: 0.05}, now) || !Fired(r2, Metrics{ErrRate: 0.2}, now) {
		t.Fatal("error_rate mal evaluado")
	}
	r3 := store.AlertRule{ID: 3, Kind: "failed", Threshold: 1, WindowMin: 60, Enabled: true,
		LastFired: now.Add(-10 * time.Minute).Format(time.RFC3339)}
	if Fired(r3, Metrics{Failed: 9}, now) {
		t.Fatal("cooldown debe frenar")
	}
	r3.LastFired = now.Add(-2 * time.Hour).Format(time.RFC3339)
	if !Fired(r3, Metrics{Failed: 9}, now) {
		t.Fatal("ventana pasada sí dispara")
	}
	r3.LastFired = "no-es-fecha"
	if !Fired(r3, Metrics{Failed: 9}, now) {
		t.Fatal("LastFired inválido no bloquea")
	}
}

func TestFiredKinds(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		kind string
		m    Metrics
		want float64
	}{
		{"latency_p95", Metrics{P95ms: 120}, 120},
		{"memory_mb", Metrics{MemoryMB: 512}, 512},
		{"down_min", Metrics{DownMin: 15}, 15},
		{"disk_pct", Metrics{DiskPct: 91}, 91},
		{"cpu_pct", Metrics{CPUPct: 88}, 88},
		{"mem_pct", Metrics{MemPct: 93}, 93},
		{"temp_c", Metrics{TempC: 82}, 82},
		{"deploy_failed", Metrics{DeployFailed: 3}, 3},
	}
	for _, c := range cases {
		r := store.AlertRule{ID: 9, Kind: c.kind, Threshold: c.want - 1, WindowMin: 60, Enabled: true}
		if !Fired(r, c.m, now) {
			t.Fatalf("%s no disparó", c.kind)
		}
		if c.m.Value(c.kind) != c.want {
			t.Fatalf("%s Value = %v", c.kind, c.m.Value(c.kind))
		}
	}
	rc := store.AlertRule{Kind: "cert_days", Threshold: 10, Enabled: true}
	if !Fired(rc, Metrics{CertDays: 7}, now) || (Metrics{CertDays: 7}).Value("cert_days") != 7 {
		t.Fatal("cert_days debe disparar con días restantes <= umbral")
	}
	rd := store.AlertRule{Kind: "deploy_failed", Threshold: 2, Enabled: true}
	if !Fired(rd, Metrics{DeployFailed: 2}, now) || (Metrics{DeployFailed: 2}).Value("deploy_failed") != 2 {
		t.Fatal("deploy_failed mal evaluado")
	}
	r := store.AlertRule{Kind: "otro", Threshold: 0, Enabled: true}
	if Fired(r, Metrics{}, now) || (Metrics{}).Value("otro") != 0 {
		t.Fatal("kind desconocido no dispara")
	}
}

func TestDescribe(t *testing.T) {
	for _, kind := range []string{"failed", "error_rate", "latency_p95", "memory_mb", "down_min", "disk_pct"} {
		if Describe(store.AlertRule{ID: 7, Kind: kind, Threshold: 3}, Metrics{}) == "" {
			t.Fatalf("describe %s vacío", kind)
		}
	}
}
