package app

import (
	"context"
	"io"
	"log/slog"
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

// TestCancelDeploy: cancela un deploy registrado, false si no existe.
func TestCancelDeploy(t *testing.T) {
	p := New(t.TempDir()+"/rkt.db", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if p.cancelDeploy(7) {
		t.Fatal("sin registro debe dar false")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancelMu.Lock()
	if p.deployCancels == nil {
		p.deployCancels = map[int64]context.CancelFunc{}
	}
	p.deployCancels[7] = cancel
	p.cancelMu.Unlock()
	if !p.cancelDeploy(7) {
		t.Fatal("registrado debe dar true")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("cancel no propagó al contexto")
	}
}

// TestMaybeAutoRollbackGuardas: sin deploy reciente o ya en rollback, no
// hace nada (ni toca rollingBack ni llama al servicio).
func TestMaybeAutoRollbackGuardas(t *testing.T) {
	p := New(t.TempDir()+"/rkt.db", slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.maybeAutoRollback("nunca-desplegada")
	p.deployMu.Lock()
	if len(p.rollingBack) != 0 {
		p.deployMu.Unlock()
		t.Fatal("sin deploy no debe marcar rollback")
	}
	p.deployMu.Unlock()
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
