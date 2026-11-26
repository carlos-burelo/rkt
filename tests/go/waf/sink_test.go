package waf

import "testing"

// El sink recibe cada bloqueo (persistencia del mapa de ataques); nil lo apaga.
func TestSinkRecibeBloqueos(t *testing.T) {
	var got []Block
	SetSink(func(b Block) { got = append(got, b) })
	defer SetSink(nil)
	record("9.9.9.9", "a.dev", "GET", "/.env", "/.env")
	if len(got) != 1 || got[0].IP != "9.9.9.9" || got[0].Rule != "/.env" || got[0].At == "" {
		t.Fatalf("sink: %+v", got)
	}
	SetSink(nil)
	record("9.9.9.9", "a.dev", "GET", "/x", "r") // sin sink no debe fallar
	if len(got) != 1 {
		t.Fatal("sin sink no se entrega nada")
	}
}
