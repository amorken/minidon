package index

import (
	"context"

	"github.com/amorken/minidon/internal/model"
)

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
