package index_test

import (
	"context"
	"testing"

	"github.com/amorken/minidon/internal/index"
	"github.com/amorken/minidon/internal/model"
)

func TestNoopIndex(t *testing.T) {
	idx := &index.NoopIndex{}

	statuses := []model.Status{
		{ID: "1", Content: "hello"},
	}

	err := idx.Index(statuses)
	if err != nil {
		t.Fatalf("unexpected error for NoopIndex.Index: %v", err)
	}

	res, err := idx.Search(context.Background(), "hello", index.SearchOptions{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error for NoopIndex.Search: %v", err)
	}

	if len(res.Hits) != 0 {
		t.Errorf("expected 0 hits from NoopIndex, got %d", len(res.Hits))
	}
	if res.Total != 0 {
		t.Errorf("expected 0 total hits, got %d", res.Total)
	}
	if res.Limit != 10 {
		t.Errorf("expected limit 10, got %d", res.Limit)
	}
}
