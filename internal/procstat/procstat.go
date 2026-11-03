// Package procstat: RSS y tiempo de CPU de un proceso (para monitoreo).
package procstat

import "time"

// Sample foto de consumo del proceso.
type Sample struct {
	RSSBytes uint64
	CPUTime  time.Duration
}
