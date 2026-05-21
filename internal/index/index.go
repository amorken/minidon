// Package index defines the Index interface that abstracts the full-text
// search backend used by minidon.
//
// The primary implementation is MeiliSearch (see meili.go), but the
// interface allows alternative backends (Bleve, Typesense, SQLite FTS5)
// to be plugged in without changes to the ingest pipeline or HTTP handlers.
package index

import "github.com/amorken/minidon/internal/model"

// SearchOptions represents pagination options for search queries.
type SearchOptions struct {
	Limit  int
	Offset int
}

// SearchResult represents the search hits along with pagination metadata.
type SearchResult struct {
	Hits   []model.Status `json:"hits"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// Index defines the interface for full-text search indexing and querying.
type Index interface {
	Index(statuses []model.Status) error
	Search(query string, opts SearchOptions) (SearchResult, error)
}

// NoopIndex is a search index that does nothing. Used as a fallback when MeiliSearch is disabled.
type NoopIndex struct{}

func (n *NoopIndex) Index(statuses []model.Status) error {
	return nil
}

func (n *NoopIndex) Search(query string, opts SearchOptions) (SearchResult, error) {
	return SearchResult{
		Hits:   []model.Status{},
		Total:  0,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}
