// Package index defines the interface and setup utilities for indexing and searching
// Mastodon statuses, supporting multiple backends.
package index

import (
	"context"
	"log/slog"

	"github.com/amorken/minidon/internal/model"
)

// NewFromConfig constructs a search index implementation (MeiliSearch or NoopIndex) based on configuration.
func NewFromConfig(disabled bool, url, apiKey string) Index {
	if disabled {
		slog.Info("Search functionality disabled; using Noop search index")
		return &NoopIndex{}
	}
	if url != "" {
		slog.Info("connected to MeiliSearch backend", "url", url)
		return NewMeiliIndex(url, apiKey)
	}
	slog.Info("MeiliSearch not configured; using Noop search index")
	return &NoopIndex{}
}

type SearchOptions struct {
	Limit  int
	Offset int
}

type SearchResult struct {
	Hits   []model.Status `json:"hits"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type Index interface {
	Index(statuses []model.Status) error
	Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error)
	EnsureSettings(ctx context.Context) error
}

type NoopIndex struct{}

func (n *NoopIndex) Index(statuses []model.Status) error {
	return nil
}

func (n *NoopIndex) Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error) {
	return SearchResult{
		Hits:   []model.Status{},
		Total:  0,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}

func (n *NoopIndex) EnsureSettings(ctx context.Context) error {
	return nil
}
