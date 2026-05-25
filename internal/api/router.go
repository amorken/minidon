// Package api provides HTTP request handlers and routers to expose API endpoints
// and stream updates to the frontend client.
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

type RouterConfig struct {
	StaticFS fs.FS
	Buffer   *buffer.Buffer
	Index    index.Index
	Pipeline *ingest.Pipeline
	Client   mastodon.Client
}

// NewRouter constructs a serve multiplexer registering all API routes, static handlers,
// and health/readiness checks.
func NewRouter(cfg RouterConfig) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/timeline", timelineHandler(cfg.Buffer))
	mux.HandleFunc("GET /api/search", searchHandler(cfg.Index))
	mux.HandleFunc("GET /api/stream", streamHandler(cfg.Pipeline))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if cfg.Client == nil || !cfg.Client.IsConnected() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not connected to mastodon"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	spaHandler := static.NewHandler(cfg.StaticFS)
	mux.Handle("GET /", spaHandler)

	return mux
}
