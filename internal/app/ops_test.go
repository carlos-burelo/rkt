package app

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestBeginOpSerializa: dos operaciones destructivas sobre la misma app no
// pueden solaparse; apps distintas sí.
func TestBeginOpSerializa(t *testing.T) {
	p := &Program{}
	if err := p.beginOp("a"); err != nil {
		t.Fatal(err)
	}
	if err := p.beginOp("a"); err == nil {
		t.Fatal("segunda operación sobre la misma app debe fallar")
	}
	if err := p.beginOp("b"); err != nil {
		t.Fatalf("otra app no debe bloquear: %v", err)
	}
	p.endOp("a")
	if err := p.beginOp("a"); err != nil {
		t.Fatalf("tras terminar debe permitir: %v", err)
	}
}

// TestBeginOpConcurrenteSoloUnGanador: la carrera real la resuelve el lock.
func TestBeginOpConcurrenteSoloUnGanador(t *testing.T) {
	p := &Program{}
	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.beginOp("x"); err == nil {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("ganadores = %d, quiere 1", wins)
	}
}
