// Package procstat: métricas de un proceso (RSS, I/O, handles) para monitoreo.
package procstat

import "time"

// Sample foto de consumo del proceso.
type Sample struct {
	RSSBytes     uint64
	PeakRSSBytes uint64
	VirtualBytes uint64 // pagefile/commit
	CPUTime      time.Duration
	ReadBytes    uint64 // I/O leído (disco + red)
	WriteBytes   uint64 // I/O escrito
	Handles      uint32
	Threads      uint32
	StartTime    time.Time // arranque del proceso (uptime)
}
