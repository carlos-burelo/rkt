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
