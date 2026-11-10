//go:build linux

package servermetrics

import "testing"

func TestSnapshotLinux(t *testing.T) {
	s, err := NewCollector().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.Memory.Total == 0 || s.CPU.Cores == 0 || s.Hostname == "" {
		t.Fatalf("snapshot incompleto: %+v", s)
	}
	if !s.Load.Supported {
		t.Fatal("linux debe soportar load")
	}
	if len(s.Disks) == 0 {
		t.Fatal("sin discos")
	}
}

func TestRealFS(t *testing.T) {
	if !realFS("ext4") || !realFS("overlay") || realFS("tmpfs") || realFS("proc") {
		t.Fatal("filtro de filesystems")
	}
}
