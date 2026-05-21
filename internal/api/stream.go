// Package api — SSE stream handler.
//
// GET /api/stream upgrades the connection to a Server-Sent Events stream and
// forwards new statuses posted by the ingest pipeline to the browser.
// Each event is a JSON-encoded model.Status with event type "status".
//
// Clients that disconnect are automatically unregistered from the fan-out
// to avoid goroutine leaks.
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
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Subscribe to the pipeline
		ch := pipeline.Subscribe()
		defer pipeline.Unsubscribe(ch)

		// Send initial keep-alive/comment or flush to establish connection
		fmt.Fprint(w, ": ok\n\n")
		flusher.Flush()

		slog.Debug("browser client connected to SSE stream", "addr", r.RemoteAddr)

		for {
			select {
			case <-r.Context().Done():
				slog.Debug("browser client disconnected from SSE stream", "addr", r.RemoteAddr)
				return
			case status, ok := <-ch:
				if !ok {
					slog.Info("SSE pipeline subscription channel closed")
					return
				}

				b, err := json.Marshal(status)
				if err != nil {
					slog.Error("failed to marshal status for SSE broadcast", "err", err)
					continue
				}

				_, err = fmt.Fprintf(w, "event: status\ndata: %s\n\n", string(b))
				if err != nil {
					slog.Warn("failed to write to SSE client; client probably disconnected", "err", err)
					return
				}
				flusher.Flush()
			}
		}
	}
}
