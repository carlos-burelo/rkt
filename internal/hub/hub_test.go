package hub

import (
	"testing"
)

func TestPublishSeqMonotono(t *testing.T) {
	b := New()
	if got := b.Publish(TopicStatus, "status", 1); got != 1 {
		t.Fatalf("seq = %d", got)
	}
	if got := b.Publish(TopicStatus, "status", 2); got != 2 {
		t.Fatalf("seq = %d", got)
	}
}

func TestSinceFiltraTopicoYCursor(t *testing.T) {
	b := New()
	b.Publish(TopicStatus, "status", "a")
	b.Publish(TopicDeployments, "deploy", "b")
	evs, seq, gap := b.Since(0, map[string]bool{TopicStatus: true})
	if gap || seq != 2 || len(evs) != 1 || evs[0].Topic != TopicStatus {
		t.Fatalf("since = %+v %d %v", evs, seq, gap)
	}
	evs, _, _ = b.Since(2, nil)
	if len(evs) != 0 {
		t.Fatalf("cursor al día debe vaciar, got %d", len(evs))
	}
}

func TestSinceHuecoTrasEviccion(t *testing.T) {
	b := New()
	for i := 0; i < Cap+10; i++ {
		b.Publish(TopicEvents, "e", i)
	}
	_, _, gap := b.Since(1, nil)
	if !gap {
		t.Fatal("cursor evictado debe marcar hueco")
	}
	evs, _, gap := b.Since(b.Seq(), nil)
	if gap || len(evs) != 0 {
		t.Fatalf("cursor actual: gap=%v evs=%d", gap, len(evs))
	}
}
