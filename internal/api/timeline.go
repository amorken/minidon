// Package api — timeline handler.
//
// GET /api/timeline?limit=N returns the N most-recent statuses from the
// in-memory ring buffer as a JSON array.  Default limit: 50, max: 200.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/amorken/minidon/internal/buffer"
)

func timelineHandler(buf *buffer.Buffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}

		if limit > 200 {
			limit = 200
		}

		statuses := buf.Recent(limit)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(statuses); err != nil {
			http.Error(w, "failed to encode timeline statuses", http.StatusInternalServerError)
		}
	}
}
