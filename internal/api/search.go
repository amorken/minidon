package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/amorken/minidon/internal/index"
)

func searchHandler(idx index.Index) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing search query parameter 'q'", http.StatusBadRequest)
			return
		}

		limitStr := r.URL.Query().Get("limit")
		limit := 20
		if limitStr != "" {
			var err error
			limit, err = strconv.Atoi(limitStr)
			if err != nil || limit <= 0 {
				http.Error(w, "invalid limit parameter", http.StatusBadRequest)
				return
			}
			if limit > 100 {
				limit = 100
			}
		}

		offsetStr := r.URL.Query().Get("offset")
		offset := 0
		if offsetStr != "" {
			var err error
			offset, err = strconv.Atoi(offsetStr)
			if err != nil || offset < 0 {
				http.Error(w, "invalid offset parameter", http.StatusBadRequest)
				return
			}
		}

		opts := index.SearchOptions{
			Limit:  limit,
			Offset: offset,
		}

		result, err := idx.Search(r.Context(), q, opts)
		if err != nil {
			slog.Error("search failed", "err", err, "query", q)
			http.Error(w, "search backend error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Error("failed to encode search result", "err", err)
		}
	}
}
