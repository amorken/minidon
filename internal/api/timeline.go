package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/amorken/minidon/internal/buffer"
)

func timelineHandler(buf *buffer.Buffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInt(r, "limit", 50, 1, 200)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		statuses := buf.Recent(limit)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(statuses); err != nil {
			slog.Error("failed to encode timeline", "err", err)
		}
	}
}
