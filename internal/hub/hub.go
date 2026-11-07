// Package hub: bus de eventos en memoria para el stream vivo de la flota.
// Un solo camino servidor→UI: los publishers (config reload, ticker de
// flota) publican aquí y /api/v1/fleet/stream reparte por tópico con
// cursor y replay. Buffer circular acotado; sin suscriptores no pasa nada.
package hub

import (
	"sync"
	"time"
)

// Cap historial por conexión lenta: basta para replays de reconexión.
const Cap = 512

// Tópicos del stream de flota.
const (
	TopicStatus      = "status"
	TopicDeployments = "deployments"
	TopicEvents      = "events"
	TopicMetas       = "metas"
)

// Event unidad del bus. Seq monótono; Data es JSON-marshalable.
type Event struct {
	Seq   int64  `json:"seq"`
	Topic string `json:"topic"`
	Type  string `json:"type"`
	At    string `json:"at"`
	Data  any    `json:"data"`
}

// Bus anillo thread-safe con secuencia global.
type Bus struct {
	mu   sync.Mutex
	seq  int64
	ring []Event
}

// New crea el bus vacío.
func New() *Bus { return &Bus{} }

// Publish añade y devuelve el seq. Nunca bloquea al llamante.
func (b *Bus) Publish(topic, typ string, data any) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	b.ring = append(b.ring, Event{
		Seq:   b.seq,
		Topic: topic,
		Type:  typ,
		At:    time.Now().UTC().Format(time.RFC3339),
		Data:  data,
	})
	if len(b.ring) > Cap {
		b.ring = append(b.ring[:0:0], b.ring[len(b.ring)-Cap:]...)
	}
	return b.seq
}

// Seq actual (para hello).
func (b *Bus) Seq() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.seq
}

// Since devuelve eventos con seq>cursor filtrados por tópico, el seq
// actual y si hubo hueco (cursor evictado o de otra época: el servidor
// reinició y su seq volvió a 0; el cliente debe pedir snapshots).
// topics vacío = todos.
func (b *Bus) Since(cursor int64, topics map[string]bool) (out []Event, seq int64, gap bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cursor > b.seq {
		return nil, b.seq, true
	}
	if len(b.ring) > 0 && cursor < b.ring[0].Seq-1 {
		return nil, b.seq, true
	}
	for _, e := range b.ring {
		if e.Seq <= cursor {
			continue
		}
		if len(topics) > 0 && !topics[e.Topic] {
			continue
		}
		out = append(out, e)
	}
	return out, b.seq, false
}
