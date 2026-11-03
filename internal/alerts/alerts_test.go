package alerts

import (
	"testing"
	"time"

	"rkt-proxy/internal/store"
)

func TestFired(t *testing.T) {
	now := time.Now().UTC()
	r := store.AlertRule{ID: 1, Kind: "failed", Threshold: 2, WindowMin: 60}
	if Fired(r, 1, 0, now) {
		t.Fatal("1 < 2 no dispara")
	}
	if !Fired(r, 2, 0, now) {
		t.Fatal("2 >= 2 dispara")
	}
	r2 := store.AlertRule{ID: 2, Kind: "error_rate", Threshold: 0.1, WindowMin: 60}
	if Fired(r2, 0, 0.05, now) {
		t.Fatal("5% < 10% no dispara")
	}
	if !Fired(r2, 0, 0.2, now) {
		t.Fatal("20% >= 10% dispara")
	}
	// cooldown: disparada hace 10min con ventana 60 no re-dispara
	r3 := store.AlertRule{ID: 3, Kind: "failed", Threshold: 1, WindowMin: 60,
		LastFired: now.Add(-10 * time.Minute).Format("2006-01-02T15:04:05Z")}
	if Fired(r3, 9, 0, now) {
		t.Fatal("cooldown debe frenar")
	}
	r3.LastFired = now.Add(-2 * time.Hour).Format("2006-01-02T15:04:05Z")
	if !Fired(r3, 9, 0, now) {
		t.Fatal("ventana pasada sí dispara")
	}
	if Describe(r) == "" {
		t.Fatal("describe vacío")
	}
}

func TestFiredBordes(t *testing.T) {
	now := time.Now().UTC()
	// LastFired inválido se ignora (no bloquea)
	r := store.AlertRule{Kind: "failed", Threshold: 1, WindowMin: 60, LastFired: "no-es-fecha"}
	if !Fired(r, 1, 0, now) {
		t.Fatal("LastFired inválido no debe bloquear")
	}
	// error_rate
	r2 := store.AlertRule{Kind: "error_rate", Threshold: 0.2, WindowMin: 60}
	if Fired(r2, 99, 0.1, now) || !Fired(r2, 0, 0.2, now) {
		t.Fatal("error_rate mal evaluado")
	}
	// tipo desconocido nunca dispara
	if Fired(store.AlertRule{Kind: "otro", Threshold: 0}, 99, 1, now) {
		t.Fatal("tipo desconocido no debe disparar")
	}
	// cooldown vigente
	r3 := store.AlertRule{Kind: "failed", Threshold: 1, WindowMin: 60, LastFired: now.Add(-10 * time.Minute).Format(time.RFC3339)}
	if Fired(r3, 5, 0, now) {
		t.Fatal("cooldown debe bloquear")
	}
	if Describe(store.AlertRule{ID: 7, Kind: "failed"}) == "" {
		t.Fatal("describe failed vacío")
	}
	if d := Describe(store.AlertRule{ID: 8, Kind: "error_rate"}); d == "" {
		t.Fatal("describe error_rate vacío")
	}
}
