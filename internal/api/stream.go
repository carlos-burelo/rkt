// SSE para logs vivos: init con tail + chunks nuevos. Evita polling.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"rkt-proxy/internal/logx"
)

// streamLog sirve text/event-stream desde path. done, si no nil, se
// consulta cada tick: true cierra el stream (deploy terminado).
func streamLog(w http.ResponseWriter, r *http.Request, path string, tailN int, done func() bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "stream no soportado")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, v any) {
		b, _ := json.Marshal(v)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		fl.Flush()
	}

	tail, err := logx.Tail(path, tailN)
	if err != nil {
		tail = ""
	}
	send("init", map[string]string{"log": tail})

	f, err := os.Open(path)
	if err != nil {
		send("end", map[string]string{"status": "missing"})
		return
	}
	defer f.Close()

	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		send("end", map[string]string{"status": "seek"})
		return
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastPing := time.Now()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if done != nil && done() {
				send("end", map[string]string{"status": "done"})
				return
			}
			fi, err := f.Stat()
			if err != nil {
				return
			}
			if fi.Size() < offset {
				// rotación/truncado: reabrir desde cero
				_ = f.Close()
				f, err = os.Open(path)
				if err != nil {
					send("end", map[string]string{"status": "missing"})
					return
				}
				offset = 0
				fi, err = f.Stat()
				if err != nil {
					return
				}
			}
			if fi.Size() == offset {
				if time.Since(lastPing) > 15*time.Second {
					_, _ = fmt.Fprint(w, ": ping\n\n")
					fl.Flush()
					lastPing = time.Now()
				}
				continue
			}
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				return
			}
			buf := make([]byte, fi.Size()-offset)
			n, _ := io.ReadFull(f, buf)
			if n > 0 {
				offset += int64(n)
				send("chunk", map[string]string{"chunk": string(buf[:n])})
			}
		}
	}
}
