package logx

import (
	"path/filepath"
	"testing"
)

func TestGeoIPPathBajoData(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "rkt.db")
	want := filepath.Join(DataDir(cfg), "geoip.csv")
	if got := GeoIPPath(cfg); got != want {
		t.Fatalf("GeoIPPath = %q, quiere %q", got, want)
	}
}
