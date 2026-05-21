// Package index — MeiliSearch implementation of the Index interface.
//
// The MeiliSearch index is named "statuses".  Searchable attributes:
// content, account.acct, account.display_name, tags.name.
// Sortable: created_at.  Filterable: language, tags.name.
//
// Documents are written in batches (debounced by the ingest pipeline) to
// amortise HTTP round-trips to MeiliSearch.
package index

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/meilisearch/meilisearch-go"

	"github.com/amorken/minidon/internal/model"
)

type meiliIndex struct {
	client meilisearch.ServiceManager
	index  meilisearch.IndexManager
}

// NewMeiliIndex creates and initializes a MeiliSearch-backed Index implementation.
func NewMeiliIndex(url, apiKey string) (Index, error) {
	client := meilisearch.New(url, meilisearch.WithAPIKey(apiKey))
	idx := client.Index("statuses")

	mi := &meiliIndex{
		client: client,
		index:  idx,
	}

	if err := mi.EnsureSettings(); err != nil {
		slog.Warn("failed to apply MeiliSearch index settings; search might not work as expected", "err", err)
	}

	return mi, nil
}

// EnsureSettings applies searchable, sortable, and filterable attributes configuration idempotently.
func (m *meiliIndex) EnsureSettings() error {
	slog.Info("ensuring MeiliSearch settings for 'statuses' index")

	// Set searchable attributes
	_, err := m.index.UpdateSearchableAttributes(&[]string{
		"content",
		"account.acct",
		"account.display_name",
		"tags.name",
	})
	if err != nil {
		return fmt.Errorf("update searchable attributes: %w", err)
	}

	// Set sortable attributes
	_, err = m.index.UpdateSortableAttributes(&[]string{
		"created_at",
	})
	if err != nil {
		return fmt.Errorf("update sortable attributes: %w", err)
	}

	// Set filterable attributes
	_, err = m.index.UpdateFilterableAttributes(&[]interface{}{
		"language",
		"tags.name",
	})
	if err != nil {
		return fmt.Errorf("update filterable attributes: %w", err)
	}

	return nil
}

// Index adds a batch of statuses to the MeiliSearch index.
func (m *meiliIndex) Index(statuses []model.Status) error {
	if len(statuses) == 0 {
		return nil
	}

	_, err := m.index.AddDocuments(statuses, nil)
	if err != nil {
		return fmt.Errorf("failed to index documents to MeiliSearch: %w", err)
	}

	slog.Debug("successfully sent status batch to MeiliSearch", "count", len(statuses))
	return nil
}

// Search searches for statuses matching the given query with pagination options.
func (m *meiliIndex) Search(query string, opts SearchOptions) (SearchResult, error) {
	limit := int64(opts.Limit)
	offset := int64(opts.Offset)

	resp, err := m.index.Search(query, &meilisearch.SearchRequest{
		Limit:  limit,
		Offset: offset,
		Sort:   []string{"created_at:desc"},
	})
	if err != nil {
		return SearchResult{}, fmt.Errorf("meilisearch search error: %w", err)
	}

	hits := make([]model.Status, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		var status model.Status
		hitJSON, err := json.Marshal(hit)
		if err != nil {
			slog.Warn("failed to marshal meilisearch hit map", "err", err)
			continue
		}
		if err := json.Unmarshal(hitJSON, &status); err != nil {
			slog.Warn("failed to unmarshal meilisearch hit to model.Status", "err", err)
			continue
		}
		hits = append(hits, status)
	}

	total := resp.TotalHits
	if total == 0 {
		total = resp.EstimatedTotalHits
	}

	return SearchResult{
		Hits:   hits,
		Total:  total,
		Limit:  opts.Limit,
		Offset: opts.Offset,
	}, nil
}
