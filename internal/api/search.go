// Package api — search handler.
//
// GET /api/search?q=<query>&limit=<n>&offset=<n> proxies the query to the
// Index backend and returns matching statuses as a JSON object with
// "hits", "total", "limit", and "offset" fields.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/amorken/minidon/internal/index"
)

func searchHandler(idx index.Index) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "query parameter 'q' is required", http.StatusBadRequest)
			return
		}

		limit := 20
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 100 {
			limit = 100
		}

		offset := 0
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		result, err := idx.Search(q, index.SearchOptions{
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			http.Error(w, "search request failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			http.Error(w, "failed to encode search response", http.StatusInternalServerError)
		}
	}
}
