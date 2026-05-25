package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/amorken/minidon/internal/index"
)

func searchHandler(idx index.Index) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing search query parameter 'q'", http.StatusBadRequest)
			return
		}

		limit, err := queryInt(r, "limit", 20, 1, 100)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		offset, err := queryInt(r, "offset", 0, 0, 1000000)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
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
