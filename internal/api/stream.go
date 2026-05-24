package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/amorken/minidon/internal/ingest"
)

func streamHandler(pipeline *ingest.Pipeline) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := pipeline.Subscribe()
		defer pipeline.Unsubscribe(ch)

		// Send initial comment to flush headers and establish stream
		if _, err := fmt.Fprint(w, ": ok\n\n"); err != nil {
			return
		}
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case status, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(status)
				if err != nil {
					slog.Error("stream: failed to marshal status", "err", err)
					continue
				}
				if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", data); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
