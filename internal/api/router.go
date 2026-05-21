package api

import (
	"io/fs"
	"net/http"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/static"
)

func NewRouter(staticFS fs.FS, pipeline *ingest.Pipeline, buf *buffer.Buffer, idx index.Index) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/timeline", timelineHandler(buf))
	mux.HandleFunc("GET /api/search", searchHandler(idx))
	mux.HandleFunc("GET /api/stream", streamHandler(pipeline))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if pipeline != nil && pipeline.Connected() {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}
	})

	spaHandler := static.NewHandler(staticFS)
	mux.Handle("GET /", spaHandler)

	return mux
}
