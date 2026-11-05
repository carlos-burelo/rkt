//go:build windows

package servermetrics

import "testing"

func TestSnapshotLive(t *testing.T) {
	s, err := NewCollector().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.Memory.Total == 0 || s.CPU.Cores == 0 || len(s.Disks) == 0 {
		t.Fatalf("snapshot incompleto: %+v", s)
	}
	if s.Hostname == "" || s.OS != "windows" {
		t.Fatalf("host/os = %q/%q", s.Hostname, s.OS)
	}
	// segunda muestra calcula delta sin romper
	s2, err := NewCollector().Snapshot()
	if err != nil || s2.Memory.Percent < 0 || s2.Memory.Percent > 100 {
		t.Fatalf("segunda muestra: %+v %v", s2, err)
	}
	c := NewCollector()
	_ = c.Processes(3)
	procs := c.Processes(5)
	if len(procs) == 0 {
		t.Fatal("sin procesos")
	}
	for _, p := range procs {
		if p.PID <= 0 || p.Name == "" {
			t.Fatalf("proceso inválido: %+v", p)
		}
	}
}
