// Package api provides HTTP request handlers and routers to expose API endpoints
// and stream updates to the frontend client.
package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/amorken/minidon/internal/buffer"
	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/ingest"
	"github.com/amorken/minidon/internal/mastodon"
	"github.com/amorken/minidon/internal/static"
)

var startTime = time.Now()

type HealthResponse struct {
	Status      string `json:"status"`
	Initialized bool   `json:"initialized"`
	Uptime      string `json:"uptime"`
}

type MastodonStatus struct {
	Connected bool   `json:"connected"`
	Server    string `json:"server"`
	Stream    string `json:"stream"`
}

type MeiliSearchStatus struct {
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	URL       string `json:"url"`
	Error     string `json:"error,omitempty"`
	Stats     any    `json:"stats,omitempty"`
}

type DependenciesStatus struct {
	Mastodon    *MastodonStatus    `json:"mastodon,omitempty"`
	MeiliSearch *MeiliSearchStatus `json:"meilisearch,omitempty"`
}

type StatuszResponse struct {
	Dependencies DependenciesStatus `json:"dependencies"`
}

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
		initialized := cfg.Buffer != nil && cfg.Index != nil && cfg.Pipeline != nil && cfg.Client != nil
		status := "healthy"
		w.Header().Set("Content-Type", "application/json")
		if !initialized {
			status = "unhealthy"
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		resp := HealthResponse{
			Status:      status,
			Initialized: initialized,
			Uptime:      time.Since(startTime).Truncate(time.Second).String(),
		}
		_ = json.NewEncoder(w).Encode(resp)
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

	mux.HandleFunc("GET /statusz", func(w http.ResponseWriter, r *http.Request) {
		var mStatus *MastodonStatus
		if cfg.Client != nil {
			mStatus = &MastodonStatus{
				Connected: cfg.Client.IsConnected(),
				Server:    cfg.Client.Server(),
				Stream:    cfg.Client.Stream(),
			}
		}

		var meiliStatus *MeiliSearchStatus
		if cfg.Index != nil {
			url := cfg.Index.URL()
			enabled := url != ""
			var connected bool
			var statsErr string
			var stats any

			if enabled {
				var err error
				stats, err = cfg.Index.Stats(r.Context())
				if err != nil {
					statsErr = err.Error()
				} else {
					connected = true
				}
			}

			meiliStatus = &MeiliSearchStatus{
				Enabled:   enabled,
				Connected: connected,
				URL:       url,
				Error:     statsErr,
				Stats:     stats,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := StatuszResponse{
			Dependencies: DependenciesStatus{
				Mastodon:    mStatus,
				MeiliSearch: meiliStatus,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	spaHandler := static.NewHandler(cfg.StaticFS)
	mux.Handle("GET /", spaHandler)

	return mux
}
