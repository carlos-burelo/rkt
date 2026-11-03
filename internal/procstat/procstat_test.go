package procstat

import (
	"os"
	"testing"
	"time"
)

// TestReadPropioProceso: la métrica sale del proceso real, no de una tabla.
func TestReadPropioProceso(t *testing.T) {
	s, err := Read(os.Getpid())
	if err != nil {
		t.Fatalf("Read(propio): %v", err)
	}
	if s.RSSBytes == 0 {
		t.Fatal("RSS debe ser > 0")
	}
	if s.VirtualBytes == 0 {
		t.Fatal("memoria virtual debe ser > 0")
	}
	if s.Threads == 0 {
		t.Fatal("hilos debe ser > 0")
	}
	if s.Handles == 0 {
		t.Fatal("handles debe ser > 0")
	}
	if s.StartTime.IsZero() {
		t.Fatal("StartTime debe existir")
	}
	if age := time.Since(s.StartTime); age < 0 || age > time.Hour {
		t.Fatalf("StartTime sospechoso: %v (edad %v)", s.StartTime, age)
	}
	// la CPU acumulada nunca decrece
	first := s.CPUTime
	time.Sleep(20 * time.Millisecond)
	s2, err := Read(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if s2.CPUTime < first {
		t.Fatalf("CPU acumulada decreció: %v < %v", s2.CPUTime, first)
	}
}

func TestReadPIDInexistente(t *testing.T) {
	if _, err := Read(1 << 30); err == nil {
		t.Fatal("pid inexistente debe fallar, no inventar métricas")
	}
}
