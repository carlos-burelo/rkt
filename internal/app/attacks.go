package app

import (
	"time"

	"rkt/internal/store"
	"rkt/internal/waf"
)

// attackLoop persiste los bloqueos del WAF por lotes. El sink del WAF solo
// encola (descarta si la cola se llena: un escaneo masivo no frena el
// request ni martillea SQLite); aquí se geolocaliza y se vuelca.
func (p *Program) attackLoop(db *store.DB) {
	q := make(chan waf.Block, 2048)
	waf.SetSink(func(b waf.Block) {
		select {
		case q <- b:
		default:
		}
	})
	var batch []store.Attack
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := db.AddAttacks(batch); err != nil {
			p.log.Warn("persistir ataques", "err", err, "n", len(batch))
		}
		batch = batch[:0]
	}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case b := <-q:
			batch = append(batch, store.Attack{
				Ts: b.At, IP: b.IP, Host: b.Host, Method: b.Method, Path: b.Path,
				Rule: b.Rule, Country: p.geo.Country(b.IP),
			})
			if len(batch) >= 200 {
				flush()
			}
		case <-t.C:
			flush()
		}
	}
}
