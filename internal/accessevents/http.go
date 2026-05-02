// internal/accessevents/http.go
package accessevents

import (
	"encoding/json"
	"net/http"
)

// NewHandler returns an http.Handler that streams access events as NDJSON on
// GET /events (or whichever path is registered). Each connected client gets
// an independent per-client channel; slow clients have events dropped rather
// than back-pressuring the engine or other clients.
func NewHandler(e *Engine) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported by this server", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ch := e.Subscribe()
		defer e.Unsubscribe(ch)

		enc := json.NewEncoder(w)
		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					// Channel closed (engine shutting down or unsubscribed externally).
					return
				}
				if err := enc.Encode(evt); err != nil {
					// Client disconnected or write failed; exit silently.
					return
				}
				flusher.Flush()
			case <-r.Context().Done():
				// Client disconnected normally; no error to log.
				return
			}
		}
	})
}
