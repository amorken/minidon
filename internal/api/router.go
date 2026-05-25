package api

import (
	"io/fs"
	"net/http"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/static"
)

// NewRouter constructs a serve multiplexer registering all API routes, static handlers,
// and health/readiness checks.
func NewRouter(
	staticFS fs.FS,
	buf *buffer.Buffer,
	idx index.Index,
	pipeline *ingest.Pipeline,
	mClient mastodon.Client,
) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/timeline", timelineHandler(buf))
	mux.HandleFunc("GET /api/search", searchHandler(idx))
	mux.HandleFunc("GET /api/stream", streamHandler(pipeline))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if mClient == nil || !mClient.IsConnected() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not connected to mastodon"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	spaHandler := static.NewHandler(staticFS)
	mux.Handle("GET /", spaHandler)

	return mux
}
