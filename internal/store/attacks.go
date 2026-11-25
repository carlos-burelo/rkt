package store

import (
	"context"
	"fmt"
	"time"
)

// Attack un bloqueo del WAF persistido (insumo del mapa de ataques).
type Attack struct {
	Ts      string `json:"ts"`
	IP      string `json:"ip"`
	Host    string `json:"host"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Rule    string `json:"rule"`
	Country string `json:"country"`
}

// Count par clave/total para los rankings.
type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// AttackSummary agregados de una ventana para el mapa y los agentes.
type AttackSummary struct {
	Hours     int      `json:"hours"`
	Total     int      `json:"total"`
	Countries []Count  `json:"countries"` // Key = ISO-3166 alpha-2; "??" = sin geolocalizar
	IPs       []Count  `json:"ips"`
	Rules     []Count  `json:"rules"`
	Hourly    []Count  `json:"hourly"` // Key = hora UTC RFC3339 truncada
	Recent    []Attack `json:"recent"`
}

// AddAttacks inserta un lote en una sola transacción. El WAF encola y un
// escritor vuelca por lotes: un escaneo masivo no hace un INSERT por request.
func (d *DB) AddAttacks(batch []Attack) error {
	if len(batch) == 0 {
		return nil
	}
	return d.WithTx(context.Background(), func(tx *Tx) error {
		for _, a := range batch {
			if _, err := tx.Exec(`INSERT INTO attacks(ts, ip, host, method, path, rule, country) VALUES(?,?,?,?,?,?,?)`,
				a.Ts, a.IP, a.Host, a.Method, a.Path, a.Rule, a.Country); err != nil {
				return err
			}
		}
		return nil
	})
}

// PruneAttacks borra ataques anteriores a before.
func (d *DB) PruneAttacks(before time.Time) (int64, error) {
	res, err := d.sql.Exec(`DELETE FROM attacks WHERE ts < ?`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SummarizeAttacks agrega la ventana [now-hours, now]. top limita cada ranking.
func (d *DB) SummarizeAttacks(hours, top int, now time.Time) (AttackSummary, error) {
	if hours <= 0 {
		hours = 24
	}
	if top <= 0 {
		top = 10
	}
	since := now.Add(-time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339)
	out := AttackSummary{Hours: hours, Countries: []Count{}, IPs: []Count{}, Rules: []Count{}, Hourly: []Count{}, Recent: []Attack{}}
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM attacks WHERE ts >= ?`, since).Scan(&out.Total); err != nil {
		return out, err
	}
	group := func(expr string, limit int, order string) ([]Count, error) {
		rows, err := d.sql.Query(fmt.Sprintf(
			`SELECT %s AS k, COUNT(*) AS n FROM attacks WHERE ts >= ? GROUP BY k ORDER BY %s LIMIT ?`, expr, order), since, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		list := []Count{}
		for rows.Next() {
			var c Count
			if err := rows.Scan(&c.Key, &c.Count); err != nil {
				return nil, err
			}
			list = append(list, c)
		}
		return list, rows.Err()
	}
	var err error
	if out.Countries, err = group(`CASE WHEN country='' THEN '??' ELSE country END`, top, `n DESC`); err != nil {
		return out, err
	}
	if out.IPs, err = group(`ip`, top, `n DESC`); err != nil {
		return out, err
	}
	if out.Rules, err = group(`rule`, top, `n DESC`); err != nil {
		return out, err
	}
	if out.Hourly, err = group(`substr(ts,1,13)||':00:00Z'`, hours+1, `k ASC`); err != nil {
		return out, err
	}
	rows, err := d.sql.Query(`SELECT ts, ip, host, method, path, rule, country FROM attacks WHERE ts >= ? ORDER BY id DESC LIMIT 50`, since)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Attack
		if err := rows.Scan(&a.Ts, &a.IP, &a.Host, &a.Method, &a.Path, &a.Rule, &a.Country); err != nil {
			return out, err
		}
		out.Recent = append(out.Recent, a)
	}
	return out, rows.Err()
}
