package store

import (
	"testing"
	"time"
)

func TestAttacksResumenYPoda(t *testing.T) {
	db := newDB(t)
	now := time.Date(2026, 11, 25, 12, 30, 0, 0, time.UTC)
	ts := func(h int) string { return now.Add(-time.Duration(h) * time.Hour).Format(time.RFC3339) }
	batch := []Attack{
		{Ts: ts(0), IP: "1.1.1.1", Host: "a.dev", Method: "GET", Path: "/.env", Rule: "/.env", Country: "AU"},
		{Ts: ts(0), IP: "1.1.1.1", Host: "a.dev", Method: "GET", Path: "/.env", Rule: "/.env", Country: "AU"},
		{Ts: ts(1), IP: "8.8.8.8", Host: "a.dev", Method: "TRACE", Path: "/", Rule: "método TRACE", Country: "US"},
		{Ts: ts(2), IP: "9.9.9.9", Host: "a.dev", Method: "GET", Path: "/x", Rule: "/.env", Country: ""},
		{Ts: ts(100), IP: "7.7.7.7", Host: "a.dev", Method: "GET", Path: "/viejo", Rule: "/.env", Country: "FR"},
	}
	if err := db.AddAttacks(batch); err != nil {
		t.Fatal(err)
	}
	if err := db.AddAttacks(nil); err != nil {
		t.Fatal("lote vacío no debe fallar")
	}
	sum, err := db.SummarizeAttacks(24, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total != 4 {
		t.Fatalf("total = %d (la fila de hace 100 h queda fuera)", sum.Total)
	}
	if sum.Countries[0].Key != "AU" || sum.Countries[0].Count != 2 {
		t.Fatalf("países = %+v", sum.Countries)
	}
	hasUnknown := false
	for _, c := range sum.Countries {
		if c.Key == "??" {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Fatalf("falta ?? para sin geolocalizar: %+v", sum.Countries)
	}
	if sum.IPs[0].Key != "1.1.1.1" || sum.Rules[0].Key != "/.env" || sum.Rules[0].Count != 3 {
		t.Fatalf("rankings: %+v %+v", sum.IPs, sum.Rules)
	}
	if len(sum.Hourly) != 3 || len(sum.Recent) != 4 {
		t.Fatalf("hourly=%d recent=%d", len(sum.Hourly), len(sum.Recent))
	}
	n, err := db.PruneAttacks(now.Add(-48 * time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("poda = %d %v", n, err)
	}
}
