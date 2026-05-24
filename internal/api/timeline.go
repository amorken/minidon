package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/amorken/minidon/internal/buffer"
)

func timelineHandler(buf *buffer.Buffer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			var err error
			limit, err = strconv.Atoi(limitStr)
			if err != nil || limit <= 0 {
				http.Error(w, "invalid limit parameter", http.StatusBadRequest)
				return
			}
			if limit > 200 {
				limit = 200
			}
		}

		statuses := buf.Recent(limit)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(statuses); err != nil {
			slog.Error("failed to encode timeline", "err", err)
		}
	}
}
